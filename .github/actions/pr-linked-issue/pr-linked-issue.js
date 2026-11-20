// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

/**
 * Linked issue policy
 *
 * A pull request from an outside contributor must be linked to an open issue
 * that carries the `help wanted` label. This script adds the `missing-issue`
 * and `stale` labels to a pull request that has no such issue, and explains
 * the policy in a comment. The stale workflow closes a pull request that
 * keeps the `missing-issue` label.
 */

const QUALIFYING_ISSUE_LABEL_NAME = 'help wanted';
const MISSING_ISSUE_LABEL_NAME = 'missing-issue';
const EXEMPT_LABEL_NAME = 'no-issue-needed';
const STALE_LABEL_NAME = 'stale';
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

function missingIssueComment(problems) {
  return `${POLICY_COMMENT_MARKER}
This pull request is labelled \`${MISSING_ISSUE_LABEL_NAME}\` because it is not linked to an open issue that carries the \`${QUALIFYING_ISSUE_LABEL_NAME}\` label:

${problems.map(problem => `- ${problem}`).join('\n')}

Jaeger accepts a pull request only for an issue that a maintainer has marked \`${QUALIFYING_ISSUE_LABEL_NAME}\`, as described in [Link each pull request to an issue](${POLICY_URL}). To resolve this, link the pull request to such an issue with a closing keyword in the description, for example \`Fixes #123\`. If the issue is not labelled \`${QUALIFYING_ISSUE_LABEL_NAME}\`, ask on the issue whether the maintainers would accept a contribution.

The pull request is also labelled \`${STALE_LABEL_NAME}\` and is closed automatically after about a week without activity unless such an issue is linked. A maintainer can exempt the pull request with the \`${EXEMPT_LABEL_NAME}\` label.`;
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

/**
 * Apply the policy to one pull request, to the pull requests linked to one
 * issue of this repository, or to every open pull request when neither number
 * is given.
 *
 * @returns {Promise<{flaggedPullRequests: number[], unflaggedPullRequests: number[]}>}
 *   The pull requests that violate the policy, and the ones that carried the
 *   `missing-issue` label and no longer violate it.
 */
async function processLinkedIssuePolicy(
  octokit, owner, repo, { pullRequestNumber, issueNumber, dryRun = false } = {}, logger = console
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
  for (const pr of pullRequests) {
    const number = pr.number;
    const login = pr.author?.login;
    const presentLabelNames = labelNames(pr.labels);
    const isFlagged = presentLabelNames.includes(MISSING_ISSUE_LABEL_NAME);
    const problems = linkedIssueProblems(pr, repository, owner);
    const violatesPolicy = problems.length > 0 &&
      !presentLabelNames.includes(EXEMPT_LABEL_NAME) &&
      !DEPENDENCY_BOT_LOGINS.includes(login) &&
      !(login && await authorHasWriteAccess(octokit, owner, repo, login, permissionsByLogin));

    if (violatesPolicy) {
      flaggedPullRequests.push(number);
      logger.log(`PR #${number} by @${login}: ${problems.join('; ')}`);
      if (dryRun) continue;
      const body = missingIssueComment(problems);
      // A newly flagged pull request always gets a new comment, because GitHub
      // notifies the author of a new comment and not of an edited one. The
      // comment is written before the labels, so that the stale label is the
      // last change and the stale workflow counts idle days from it.
      if (isFlagged) {
        await rewritePolicyComment(octokit, owner, repo, number, body);
      } else {
        await octokit.rest.issues.createComment({ owner, repo, issue_number: number, body });
      }
      const labels = [MISSING_ISSUE_LABEL_NAME, STALE_LABEL_NAME]
        .filter(name => !presentLabelNames.includes(name));
      if (labels.length > 0) {
        await octokit.rest.issues.addLabels({ owner, repo, issue_number: number, labels });
      }
    } else if (isFlagged) {
      unflaggedPullRequests.push(number);
      logger.log(`PR #${number} by @${login}: meets the policy, removing ${MISSING_ISSUE_LABEL_NAME}`);
      if (dryRun) continue;
      await rewritePolicyComment(octokit, owner, repo, number, policyMetComment());
      // missing-issue is removed last, so that a run that fails in between
      // still finds the pull request flagged and finishes the work.
      for (const name of [STALE_LABEL_NAME, MISSING_ISSUE_LABEL_NAME]) {
        if (presentLabelNames.includes(name)) {
          await octokit.rest.issues.removeLabel({ owner, repo, issue_number: number, name });
        }
      }
    }
  }
  return { flaggedPullRequests, unflaggedPullRequests };
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
  github, core, owner, repo, pullRequestNumber, issueNumber, dryRun = false
}) {
  try {
    const result = await processLinkedIssuePolicy(github, owner, repo, {
      pullRequestNumber: parseOptionalNumber(pullRequestNumber, 'pull-request-number'),
      issueNumber: parseOptionalNumber(issueNumber, 'issue-number'),
      dryRun
    });
    const prefix = dryRun ? 'Dry-run plan: ' : '';
    core.info(`${prefix}PRs that violate the policy: ${result.flaggedPullRequests.join(', ') || 'none'}`);
    core.info(`${prefix}PRs that now meet the policy: ${result.unflaggedPullRequests.join(', ') || 'none'}`);
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
module.exports.EXEMPT_LABEL_NAME = EXEMPT_LABEL_NAME;
module.exports.STALE_LABEL_NAME = STALE_LABEL_NAME;
