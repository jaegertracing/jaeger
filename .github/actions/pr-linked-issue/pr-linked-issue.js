// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

/**
 * Linked issue policy
 *
 * A pull request from an outside contributor must be linked to an open issue
 * that carries the `help wanted` label. This script flags a pull request that
 * has no such issue: it adds one of the two flag labels, `missing-issue` or
 * `unapproved-issue`, and explains the policy in a comment. A run that is asked
 * to close expired pull requests closes one that has been flagged for a week.
 */

const QUALIFYING_ISSUE_LABEL_NAME = 'help wanted';
// The two flag labels separate what the pull request waits for. Under
// `missing-issue` the author has to link an issue. Under `unapproved-issue` the
// linked issue is open, and a maintainer decides whether it gets `help wanted`.
const MISSING_ISSUE_LABEL_NAME = 'missing-issue';
const UNAPPROVED_ISSUE_LABEL_NAME = 'unapproved-issue';
const FLAG_LABEL_NAMES = [MISSING_ISSUE_LABEL_NAME, UNAPPROVED_ISSUE_LABEL_NAME];
const EXEMPT_LABEL_NAME = 'no-issue-needed';
// The stale workflow keeps a pull request with this label open, so a
// maintainer who applies it expects the same from this policy.
const DO_NOT_EXPIRE_LABEL_NAME = 'do-not-expire';
// The days count from the policy comment, which is posted when the pull request
// is flagged. Activity on the pull request does not extend them, and neither
// does a rewrite of the comment, which keeps its creation time.
const DAYS_FROM_FLAG_TO_CLOSE = 7;
const MILLISECONDS_PER_DAY = 24 * 60 * 60 * 1000;
const POLICY_URL =
  'https://github.com/jaegertracing/jaeger/blob/main/CONTRIBUTING_GUIDELINES.md#link-each-pull-request-to-an-issue';
// The exemption names the bots instead of exempting every bot account,
// because a coding agent can also open a pull request as a bot.
const DEPENDENCY_BOT_LOGINS = ['dependabot', 'renovate-bot'];
const POLICY_COMMENT_MARKER = '<!-- jaeger-pr-linked-issue -->';
// The workflow posts the policy comment as github-actions[bot], and a
// maintainer who applies the policy by hand posts it under their own account.
// A marker in a comment from anyone else is ignored, so that pasting the
// marker does not make the action rewrite that comment.
const WORKFLOW_ACCOUNT_LOGIN = 'github-actions[bot]';
const MAINTAINER_AUTHOR_ASSOCIATIONS = ['OWNER', 'MEMBER', 'COLLABORATOR'];

const pullRequestFields = `
  fragment pullRequestFields on PullRequest {
    number
    state
    author { login }
    labels(first: 100) { nodes { name } }
    closingIssuesReferences(first: 20) {
      nodes {
        number
        state
        repository { nameWithOwner owner { login } }
        labels(first: 100) { nodes { name } }
      }
    }
  }
`;

const openPullRequestsQuery = `
  query openPullRequests($owner: String!, $repo: String!, $cursor: String) {
    repository(owner: $owner, name: $repo) {
      pullRequests(states: OPEN, first: 100, after: $cursor) {
        nodes { ...pullRequestFields }
        pageInfo { hasNextPage endCursor }
      }
    }
  }
  ${pullRequestFields}
`;

const pullRequestQuery = `
  query pullRequest($owner: String!, $repo: String!, $number: Int!) {
    repository(owner: $owner, name: $repo) {
      pullRequest(number: $number) { ...pullRequestFields }
    }
  }
  ${pullRequestFields}
`;

function labelNames(labels) {
  return labels.nodes.map(label => label.name);
}

async function fetchOpenPullRequests(octokit, owner, repo) {
  const pullRequests = [];
  let cursor = null;
  do {
    const result = await octokit.graphql(openPullRequestsQuery, { owner, repo, cursor });
    const connection = result.repository.pullRequests;
    pullRequests.push(...connection.nodes);
    cursor = connection.pageInfo.hasNextPage ? connection.pageInfo.endCursor : null;
  } while (cursor);
  return pullRequests;
}

async function fetchPullRequestIfOpen(octokit, owner, repo, number) {
  const result = await octokit.graphql(pullRequestQuery, { owner, repo, number });
  const pr = result.repository.pullRequest;
  return pr.state === 'OPEN' ? pr : undefined;
}

/**
 * Say why a linked issue does not qualify, or return undefined when it does.
 * An issue in any repository of the same organization qualifies, because
 * work on one Jaeger repository is often tracked by an issue in another.
 */
function disqualification(issue, owner) {
  if (issue.repository.owner.login !== owner) return 'belongs to another organization';
  if (issue.state !== 'OPEN') return 'is closed';
  if (!labelNames(issue.labels).includes(QUALIFYING_ISSUE_LABEL_NAME)) {
    return `is not labelled \`${QUALIFYING_ISSUE_LABEL_NAME}\``;
  }
  return undefined;
}

/**
 * The permission comes from the collaborators API and not from the pull
 * request's author association, which reports a private member of the
 * organization as an outside contributor.
 */
async function authorHasWriteAccess(octokit, owner, repo, login, permissionsByLogin) {
  if (!permissionsByLogin.has(login)) {
    const { data } = await octokit.rest.repos.getCollaboratorPermissionLevel({ owner, repo, username: login });
    permissionsByLogin.set(login, data.permission);
  }
  return ['admin', 'write'].includes(permissionsByLogin.get(login));
}

function issueReference(issue, repository) {
  return issue.repository.nameWithOwner === repository
    ? `#${issue.number}`
    : `${issue.repository.nameWithOwner}#${issue.number}`;
}

/**
 * List what is wrong with the issue links of a pull request, one entry per
 * linked issue. The list is empty when a linked issue qualifies.
 */
function linkedIssueProblems(pr, repository, owner) {
  const issues = pr.closingIssuesReferences.nodes;
  if (issues.length === 0) return ['no issue is linked'];
  if (issues.some(issue => !disqualification(issue, owner))) return [];
  return issues.map(issue => `${issueReference(issue, repository)} ${disqualification(issue, owner)}`);
}

/**
 * Choose the flag label of a pull request that violates the policy. Every
 * linked issue of such a pull request is disqualified, so an open issue of
 * this organization is one that lacks `help wanted`.
 */
function applicableFlagLabelName(pr, owner) {
  const hasOpenOrganizationIssue = pr.closingIssuesReferences.nodes.some(issue =>
    issue.repository.owner.login === owner && issue.state === 'OPEN'
  );
  return hasOpenOrganizationIssue ? UNAPPROVED_ISSUE_LABEL_NAME : MISSING_ISSUE_LABEL_NAME;
}

function policyViolatedComment(flagLabelName, problems) {
  const resolution = flagLabelName === UNAPPROVED_ISSUE_LABEL_NAME
    ? `To resolve this, ask on the linked issue whether the maintainers would accept a contribution. A maintainer who accepts it adds the \`${QUALIFYING_ISSUE_LABEL_NAME}\` label to the issue.`
    : 'To resolve this, link the pull request to such an issue with a closing keyword in the description, for example `Fixes #123`.';
  return `${POLICY_COMMENT_MARKER}
This pull request is labelled \`${flagLabelName}\` because it is not linked to an open issue that carries the \`${QUALIFYING_ISSUE_LABEL_NAME}\` label:

${problems.map(problem => `- ${problem}`).join('\n')}

Jaeger accepts a pull request only for an issue that a maintainer has marked \`${QUALIFYING_ISSUE_LABEL_NAME}\`, as described in [Link each pull request to an issue](${POLICY_URL}). ${resolution}

The pull request is closed automatically ${DAYS_FROM_FLAG_TO_CLOSE} days after this comment was posted unless it is linked to such an issue by then. A maintainer can exempt the pull request with the \`${EXEMPT_LABEL_NAME}\` label.`;
}

function pullRequestClosedComment() {
  return `This pull request has been automatically closed because it is not linked to an open issue that carries the \`${QUALIFYING_ISSUE_LABEL_NAME}\` label. See [Link each pull request to an issue](${POLICY_URL}). You may reopen it once its description links such an issue.`;
}

function policyMetComment() {
  return `${POLICY_COMMENT_MARKER}
This pull request now meets the linked issue policy.`;
}

function isPolicyComment(comment) {
  return comment.body?.includes(POLICY_COMMENT_MARKER) &&
    (comment.user?.login === WORKFLOW_ACCOUNT_LOGIN ||
      MAINTAINER_AUTHOR_ASSOCIATIONS.includes(comment.author_association));
}

async function findLatestPolicyComment(octokit, owner, repo, number) {
  const comments = await octokit.paginate(octokit.rest.issues.listComments, {
    owner, repo, issue_number: number, per_page: 100
  });
  return comments.findLast(isPolicyComment);
}

// The latest policy comment is rewritten in place, so that a pull request
// carries one current statement of its state and not a history of them.
async function rewritePolicyComment(octokit, owner, repo, number, body) {
  const comment = await findLatestPolicyComment(octokit, owner, repo, number);
  if (comment && comment.body !== body) {
    await octokit.rest.issues.updateComment({ owner, repo, comment_id: comment.id, body });
  }
}

async function wasFlaggedLongEnoughToClose(octokit, owner, repo, number, now) {
  const comment = await findLatestPolicyComment(octokit, owner, repo, number);
  return comment !== undefined &&
    now - new Date(comment.created_at) >= DAYS_FROM_FLAG_TO_CLOSE * MILLISECONDS_PER_DAY;
}

/**
 * Apply the policy to one pull request, to the pull requests linked to one
 * issue of this repository, or to every open pull request when neither number
 * is given.
 *
 * With closeExpiredPullRequests the run checks only the pull requests that
 * already carry a flag label, and closes each expired one: a pull request
 * that still violates the policy DAYS_FROM_FLAG_TO_CLOSE days after it was
 * flagged. Such a run flags no pull request, so the days always start at a
 * check that an event started, which posts the policy comment.
 *
 * @returns {Promise<{flaggedPullRequests: number[], unflaggedPullRequests: number[], closedPullRequests: number[]}>}
 *   The pull requests that violate the policy, the ones that carried a flag
 *   label and no longer violate it, and the ones that the run closed.
 */
async function processLinkedIssuePolicy(
  octokit, owner, repo,
  { pullRequestNumber, issueNumber, closeExpiredPullRequests = false, dryRun = false, now = new Date() } = {},
  logger = console
) {
  const repository = `${owner}/${repo}`;
  let pullRequests;
  if (pullRequestNumber) {
    const pr = await fetchPullRequestIfOpen(octokit, owner, repo, pullRequestNumber);
    pullRequests = pr ? [pr] : [];
  } else {
    pullRequests = (await fetchOpenPullRequests(octokit, owner, repo)).filter(pr =>
      !issueNumber || pr.closingIssuesReferences.nodes.some(issue =>
        issueReference(issue, repository) === `#${issueNumber}`
      )
    );
  }

  const permissionsByLogin = new Map();
  const flaggedPullRequests = [];
  const unflaggedPullRequests = [];
  const closedPullRequests = [];
  for (const pr of pullRequests) {
    const number = pr.number;
    const login = pr.author?.login;
    const presentLabelNames = labelNames(pr.labels);
    const presentFlagLabelNames = FLAG_LABEL_NAMES.filter(name => presentLabelNames.includes(name));
    const isFlagged = presentFlagLabelNames.length > 0;
    if (closeExpiredPullRequests && !isFlagged) continue;
    const problems = linkedIssueProblems(pr, repository, owner);
    const violatesPolicy = problems.length > 0 &&
      !presentLabelNames.includes(EXEMPT_LABEL_NAME) &&
      !DEPENDENCY_BOT_LOGINS.includes(login) &&
      !(login && await authorHasWriteAccess(octokit, owner, repo, login, permissionsByLogin));

    if (violatesPolicy) {
      const flagLabelName = applicableFlagLabelName(pr, owner);
      flaggedPullRequests.push(number);
      logger.log(`PR #${number} by @${login}: ${flagLabelName}: ${problems.join('; ')}`);
      if (!dryRun) {
        const body = policyViolatedComment(flagLabelName, problems);
        // A newly flagged pull request always gets a new comment, because
        // GitHub notifies the author of a new comment and not of an edited
        // one.
        if (isFlagged) {
          await rewritePolicyComment(octokit, owner, repo, number, body);
        } else {
          await octokit.rest.issues.createComment({ owner, repo, issue_number: number, body });
        }
        if (!presentLabelNames.includes(flagLabelName)) {
          await octokit.rest.issues.addLabels({ owner, repo, issue_number: number, labels: [flagLabelName] });
        }
        // The other flag label is removed after the new one is added, so that
        // a run that fails in between still finds the pull request flagged.
        for (const name of presentFlagLabelNames.filter(name => name !== flagLabelName)) {
          await octokit.rest.issues.removeLabel({ owner, repo, issue_number: number, name });
        }
      }
      if (closeExpiredPullRequests && !presentLabelNames.includes(DO_NOT_EXPIRE_LABEL_NAME) &&
        await wasFlaggedLongEnoughToClose(octokit, owner, repo, number, now)) {
        closedPullRequests.push(number);
        logger.log(`PR #${number} by @${login}: flagged for ${DAYS_FROM_FLAG_TO_CLOSE} days or more, closing`);
        if (dryRun) continue;
        // The pull request is closed before the comment is posted. A run that
        // fails in between leaves a closed pull request without the comment,
        // and the other order would repeat the comment on every later run.
        await octokit.rest.pulls.update({ owner, repo, pull_number: number, state: 'closed' });
        await octokit.rest.issues.createComment({
          owner, repo, issue_number: number, body: pullRequestClosedComment()
        });
      }
    } else if (isFlagged) {
      unflaggedPullRequests.push(number);
      logger.log(`PR #${number} by @${login}: meets the policy, removing ${presentFlagLabelNames.join(', ')}`);
      if (dryRun) continue;
      await rewritePolicyComment(octokit, owner, repo, number, policyMetComment());
      // The flag labels are removed last, so that a run that fails in between
      // still finds the pull request flagged and finishes the work.
      for (const name of presentFlagLabelNames) {
        await octokit.rest.issues.removeLabel({ owner, repo, issue_number: number, name });
      }
    }
  }
  return { flaggedPullRequests, unflaggedPullRequests, closedPullRequests };
}

/**
 * An empty value means that the number was not given. Any other value must be
 * a positive integer: a malformed number that was read as "not given" would
 * turn a run for one pull request into a run for all of them.
 */
function parseOptionalNumber(value, name) {
  if (value === undefined || value === '') return undefined;
  if (!/^[1-9]\d*$/.test(value)) {
    throw new Error(`${name} must be a positive integer, got "${value}"`);
  }
  return Number(value);
}

async function githubActionHandler({
  github, core, owner, repo, pullRequestNumber, issueNumber, closeExpiredPullRequests = false, dryRun = false
}) {
  try {
    const result = await processLinkedIssuePolicy(github, owner, repo, {
      pullRequestNumber: parseOptionalNumber(pullRequestNumber, 'pull-request-number'),
      issueNumber: parseOptionalNumber(issueNumber, 'issue-number'),
      closeExpiredPullRequests,
      dryRun
    });
    const prefix = dryRun ? 'Dry-run plan: ' : '';
    core.info(`${prefix}PRs that violate the policy: ${result.flaggedPullRequests.join(', ') || 'none'}`);
    core.info(`${prefix}PRs that now meet the policy: ${result.unflaggedPullRequests.join(', ') || 'none'}`);
    core.info(`${prefix}PRs closed: ${result.closedPullRequests.join(', ') || 'none'}`);
  } catch (error) {
    core.setFailed(`Error applying the linked issue policy: ${error.message}`);
    throw error;
  }
}

module.exports = githubActionHandler;
module.exports.processLinkedIssuePolicy = processLinkedIssuePolicy;
module.exports.parseOptionalNumber = parseOptionalNumber;
module.exports.POLICY_COMMENT_MARKER = POLICY_COMMENT_MARKER;
module.exports.QUALIFYING_ISSUE_LABEL_NAME = QUALIFYING_ISSUE_LABEL_NAME;
module.exports.MISSING_ISSUE_LABEL_NAME = MISSING_ISSUE_LABEL_NAME;
module.exports.UNAPPROVED_ISSUE_LABEL_NAME = UNAPPROVED_ISSUE_LABEL_NAME;
module.exports.EXEMPT_LABEL_NAME = EXEMPT_LABEL_NAME;
module.exports.DO_NOT_EXPIRE_LABEL_NAME = DO_NOT_EXPIRE_LABEL_NAME;
