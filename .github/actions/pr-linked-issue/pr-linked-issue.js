// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

/**
 * Linked issue policy
 *
 * A pull request from an outside contributor must be linked to a qualifying
 * issue: an open issue that carries the `help wanted` label. This script adds
 * the `missing-issue` and `stale` labels to a pull request that has no
 * qualifying issue, and explains the policy in one comment. The stale workflow
 * closes a pull request that keeps the `missing-issue` label.
 */

const QUALIFYING_ISSUE_LABEL_NAME = 'help wanted';
const MISSING_ISSUE_LABEL_NAME = 'missing-issue';
const EXEMPT_LABEL_NAME = 'no-issue-needed';
const STALE_LABEL_NAME = 'stale';
const POLICY_COMMENT_MARKER = '<!-- jaeger-pr-linked-issue -->';
const POLICY_URL =
  'https://github.com/jaegertracing/jaeger/blob/main/CONTRIBUTING_GUIDELINES.md#link-each-pull-request-to-an-issue';
// The exemption names the bots instead of exempting every bot account,
// because a coding agent can also open a pull request as a bot.
const DEPENDENCY_BOT_LOGINS = ['dependabot', 'renovate', 'renovate-bot'];
// The workflow posts the policy comment as github-actions[bot], and a
// maintainer who runs the local script posts it under their own account. A
// marker in a comment from anyone else is ignored, so that pasting the marker
// does not make the action rewrite that comment.
const WORKFLOW_ACCOUNT_LOGIN = 'github-actions[bot]';
const MAINTAINER_AUTHOR_ASSOCIATIONS = ['OWNER', 'MEMBER', 'COLLABORATOR'];

// GitHub allows at most 100 labels on an issue or pull request, so the label
// connections are complete.
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

// A run for one pull request reads only that pull request, so that the cost
// of a pull request event does not grow with the number of open pull requests.
async function fetchPullRequestIfOpen(octokit, owner, repo, number) {
  const result = await octokit.graphql(pullRequestQuery, { owner, repo, number });
  const pr = result.repository.pullRequest;
  return pr?.state === 'OPEN' ? pr : undefined;
}

/**
 * An issue in any repository of the same organization qualifies, because
 * work on one Jaeger repository is often tracked by an issue in another.
 */
function isQualifyingIssue(issue, owner) {
  return issue.state === 'OPEN' &&
    issue.repository.owner.login.toLowerCase() === owner.toLowerCase() &&
    labelNames(issue.labels).includes(QUALIFYING_ISSUE_LABEL_NAME);
}

// A pull request with more than 20 linked issues is judged by the first 20,
// because treating the unread ones as qualifying would let a long list of
// closing keywords pass the policy.
function hasQualifyingIssue(pr, owner) {
  return pr.closingIssuesReferences.nodes.some(issue => isQualifyingIssue(issue, owner));
}

function isDependencyBotOrLabelledExempt(pr) {
  return DEPENDENCY_BOT_LOGINS.includes(pr.author?.login) ||
    labelNames(pr.labels).includes(EXEMPT_LABEL_NAME);
}

/**
 * The permission comes from the collaborators API and not from the pull
 * request's author association, which reports a private member of the
 * organization as an outside contributor.
 */
async function authorHasWriteAccess(octokit, owner, repo, pr, permissionsByLogin) {
  const login = pr.author?.login;
  if (!login) return false;
  if (!permissionsByLogin.has(login)) {
    const { data } = await octokit.rest.repos.getCollaboratorPermissionLevel({ owner, repo, username: login });
    permissionsByLogin.set(login, data.permission);
  }
  return ['admin', 'write'].includes(permissionsByLogin.get(login));
}

function linkedIssueReferences(pr, repository) {
  return pr.closingIssuesReferences.nodes.map(issue =>
    issue.repository.nameWithOwner === repository
      ? `#${issue.number}`
      : `${issue.repository.nameWithOwner}#${issue.number}`
  );
}

function missingIssueComment(pr, repository) {
  const references = linkedIssueReferences(pr, repository);
  const finding = references.length === 0
    ? 'No issue is linked to this pull request.'
    : `The linked issues do not qualify: ${references.join(', ')}.`;
  return `${POLICY_COMMENT_MARKER}
This pull request is labelled \`${MISSING_ISSUE_LABEL_NAME}\` because it is not linked to an open issue that carries the \`${QUALIFYING_ISSUE_LABEL_NAME}\` label. ${finding}

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
  let latest;
  for (let page = 1; ; page++) {
    const { data } = await octokit.rest.issues.listComments({
      owner,
      repo,
      issue_number: number,
      per_page: 100,
      page
    });
    latest = data.findLast(isPolicyComment) || latest;
    if (data.length < 100) return latest;
  }
}

async function createMissingPolicyLabels(octokit, owner, repo) {
  const labels = [
    [MISSING_ISSUE_LABEL_NAME, 'CFD3D7', 'Pull request is not linked to an open help wanted issue'],
    [EXEMPT_LABEL_NAME, '0E8A16', 'Pull request is accepted without a linked help wanted issue']
  ];
  for (const [name, color, description] of labels) {
    try {
      await octokit.rest.issues.getLabel({ owner, repo, name });
    } catch (error) {
      if (error.status !== 404) throw error;
      await octokit.rest.issues.createLabel({ owner, repo, name, color, description });
    }
  }
}

async function flagPullRequest(octokit, owner, repo, pr) {
  const number = pr.number;
  const body = missingIssueComment(pr, `${owner}/${repo}`);
  const presentLabelNames = labelNames(pr.labels);
  const isNewlyFlagged = !presentLabelNames.includes(MISSING_ISSUE_LABEL_NAME);
  // A newly flagged pull request always gets a new comment, because GitHub
  // notifies the author of a new comment and not of an edited one. The comment
  // is written before the labels, so that the stale label is the last change
  // that this action makes and the stale workflow counts idle days from it.
  const comment = isNewlyFlagged ? undefined : await findLatestPolicyComment(octokit, owner, repo, number);
  if (!comment) {
    await octokit.rest.issues.createComment({ owner, repo, issue_number: number, body });
  } else if (comment.body !== body) {
    await octokit.rest.issues.updateComment({ owner, repo, comment_id: comment.id, body });
  }
  const labelsToAdd = [MISSING_ISSUE_LABEL_NAME, STALE_LABEL_NAME]
    .filter(name => !presentLabelNames.includes(name));
  if (labelsToAdd.length === 0) return;
  if (isNewlyFlagged) await createMissingPolicyLabels(octokit, owner, repo);
  await octokit.rest.issues.addLabels({ owner, repo, issue_number: number, labels: labelsToAdd });
}

async function unflagPullRequest(octokit, owner, repo, pr) {
  const number = pr.number;
  const presentLabelNames = labelNames(pr.labels);
  const labelsToRemove = [MISSING_ISSUE_LABEL_NAME, STALE_LABEL_NAME]
    .filter(name => presentLabelNames.includes(name));
  for (const name of labelsToRemove) {
    try {
      await octokit.rest.issues.removeLabel({ owner, repo, issue_number: number, name });
    } catch (error) {
      // A 404 means that the label was removed after the query read it.
      if (error.status !== 404) throw error;
    }
  }
  const comment = await findLatestPolicyComment(octokit, owner, repo, number);
  if (comment && comment.body !== policyMetComment()) {
    await octokit.rest.issues.updateComment({ owner, repo, comment_id: comment.id, body: policyMetComment() });
  }
}

function linksIssue(pr, repository, issueNumber) {
  return pr.closingIssuesReferences.nodes.some(issue =>
    issue.repository.nameWithOwner === repository && issue.number === issueNumber
  );
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
    pullRequests = (await fetchOpenPullRequests(octokit, owner, repo))
      .filter(pr => !issueNumber || linksIssue(pr, repository, issueNumber));
  }

  const permissionsByLogin = new Map();
  const flaggedPullRequests = [];
  const unflaggedPullRequests = [];
  for (const pr of pullRequests) {
    const violatesPolicy = !isDependencyBotOrLabelledExempt(pr) &&
      !hasQualifyingIssue(pr, owner) &&
      !(await authorHasWriteAccess(octokit, owner, repo, pr, permissionsByLogin));
    if (violatesPolicy) {
      flaggedPullRequests.push(pr.number);
      const references = linkedIssueReferences(pr, repository).join(', ') || 'none';
      logger.log(`PR #${pr.number} by @${pr.author?.login}: no qualifying issue (linked: ${references})`);
      if (!dryRun) await flagPullRequest(octokit, owner, repo, pr);
    } else if (labelNames(pr.labels).includes(MISSING_ISSUE_LABEL_NAME)) {
      unflaggedPullRequests.push(pr.number);
      logger.log(`PR #${pr.number} by @${pr.author?.login}: meets the policy, removing ${MISSING_ISSUE_LABEL_NAME}`);
      if (!dryRun) await unflagPullRequest(octokit, owner, repo, pr);
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
    core.info(`${prefix}PRs without a qualifying issue: ${result.flaggedPullRequests.join(', ') || 'none'}`);
    core.info(`${prefix}PRs that now meet the policy: ${result.unflaggedPullRequests.join(', ') || 'none'}`);
  } catch (error) {
    core.setFailed(`Error applying the linked issue policy: ${error.message}`);
    throw error;
  }
}

module.exports = githubActionHandler;
module.exports.processLinkedIssuePolicy = processLinkedIssuePolicy;
module.exports.parseOptionalNumber = parseOptionalNumber;
module.exports.missingIssueComment = missingIssueComment;
module.exports.policyMetComment = policyMetComment;
module.exports.QUALIFYING_ISSUE_LABEL_NAME = QUALIFYING_ISSUE_LABEL_NAME;
module.exports.MISSING_ISSUE_LABEL_NAME = MISSING_ISSUE_LABEL_NAME;
module.exports.EXEMPT_LABEL_NAME = EXEMPT_LABEL_NAME;
module.exports.STALE_LABEL_NAME = STALE_LABEL_NAME;
module.exports.POLICY_COMMENT_MARKER = POLICY_COMMENT_MARKER;
