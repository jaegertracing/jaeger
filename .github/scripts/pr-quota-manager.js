#!/usr/bin/env node

// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

/**
 * PR Quota Management System
 *
 * This script implements a "Waiting Room" system that limits concurrent open PRs
 * from contributors based on their merge history, automatically unlocking queued PRs
 * when quota becomes available.
 *
 * Usage:
 *   - Via GitHub Actions (integrated with actions/github-script)
 *   - Manual execution: GITHUB_TOKEN=<token> node pr-quota-manager.js <username> [owner] [repo]
 */

const LABEL_NAME = 'pr-quota-reached';
const LABEL_COLOR = 'CFD3D7';
const DUPLICATE_LABEL_NAME = 'duplicate';
const OVERRIDE_LABEL_NAME = 'allow-multiple-prs';
const STALE_LABEL_NAME = 'stale';
const ISSUE_LIMIT_COMMENT_MARKER = '<!-- jaeger-pr-per-issue-limit -->';

const linkedIssuesQuery = `
  query linkedIssues($owner: String!, $repo: String!, $number: Int!, $after: String) {
    repository(owner: $owner, name: $repo) {
      pullRequest(number: $number) {
        closingIssuesReferences(first: 100, after: $after) {
          nodes {
            number
            repository { nameWithOwner }
          }
          pageInfo { hasNextPage endCursor }
        }
      }
    }
  }
`;

const linkedPullRequestsQuery = `
  query linkedPullRequests($owner: String!, $repo: String!, $number: Int!, $after: String) {
    repository(owner: $owner, name: $repo) {
      issue(number: $number) {
        labels(first: 100) { nodes { name } }
        closedByPullRequestsReferences(first: 100, after: $after) {
          nodes {
            number
            state
            isDraft
            createdAt
            repository { nameWithOwner }
            labels(first: 100) { nodes { name } }
          }
          pageInfo { hasNextPage endCursor }
        }
      }
    }
  }
`;

/**
 * Format open/limit counts as a bullet-point status block
 * @param {number} openCount - Number of currently open PRs
 * @param {number} quota - Allowed quota
 * @returns {string} Formatted status string
 */
function formatStatus(openCount, quota) {
  return `  * Open: ${openCount}\n  * Limit: ${quota}`;
}

/**
 * Calculate the quota for a user based on their merged PR count
 * @param {number} mergedCount - Number of merged PRs
 * @returns {number} The allowed quota
 */
function calculateQuota(mergedCount) {
  if (mergedCount === 0) return 1;
  if (mergedCount === 1) return 2;
  if (mergedCount === 2) return 3;
  return 10; // Unlimited for 3+ merged PRs
}

/**
 * Fetch open and merged PRs by a specific author
 * Optimized to stop early: fetches all open PRs and only enough merged PRs to determine quota
 * @param {object} octokit - GitHub API client
 * @param {string} owner - Repository owner/org
 * @param {string} repo - Repository name
 * @param {string} author - PR author username
 * @returns {Promise<{openPRs: Array, mergedCount: number}>} Open PRs and count of merged PRs
 */
async function fetchAuthorPRs(octokit, owner, repo, author) {
  const openPRs = [];
  const mergedPRs = [];
  const perPage = 100;
  const MAX_MERGED_NEEDED = 3; // Stop after 3 merged PRs (gives unlimited quota)

  // Fetch open PRs
  let page = 1;
  while (true) {
    const { data } = await octokit.rest.pulls.list({
      owner,
      repo,
      state: 'open',
      per_page: perPage,
      page,
      sort: 'created',
      direction: 'asc'
    });

    if (data.length === 0) break;

    const authorPRs = data.filter(pr => pr.user.login === author);
    openPRs.push(...authorPRs);

    if (data.length < perPage) break;
    page++;
  }

  // Fetch merged PRs, but stop once we have enough to determine quota
  page = 1;
  while (mergedPRs.length < MAX_MERGED_NEEDED) {
    const { data } = await octokit.rest.pulls.list({
      owner,
      repo,
      state: 'closed',
      per_page: perPage,
      page,
      sort: 'created',
      direction: 'desc' // Most recent first to find merges faster
    });

    if (data.length === 0) break;

    const authorMergedPRs = data.filter(pr => pr.user.login === author && pr.merged_at !== null);
    mergedPRs.push(...authorMergedPRs);

    // Stop if we have enough merged PRs to determine unlimited quota
    if (mergedPRs.length >= MAX_MERGED_NEEDED) break;

    if (data.length < perPage) break;
    page++;
  }

  return {
    openPRs,
    mergedCount: mergedPRs.length
  };
}

/**
 * Process quota management for a specific author
 * @param {object} octokit - GitHub API client
 * @param {string} owner - Repository owner
 * @param {string} repo - Repository name
 * @param {string} author - PR author username
 * @param {object} logger - Logger object (console or custom)
 * @param {boolean} dryRun - If true, only print actions without executing them
 * @returns {Promise<object>} Processing results
 */
async function processQuotaForAuthor(octokit, owner, repo, author, logger = console, dryRun = false) {
  if (dryRun) {
    logger.log('🔍 DRY RUN MODE - No changes will be made\n');
  }
  logger.log(`\n=== Processing Quota for: @${author} ===\n`);

  // Fetch PRs by the author (optimized to stop early)
  const { openPRs, mergedCount } = await fetchAuthorPRs(octokit, owner, repo, author);

  // Open PRs are already sorted by creation date (oldest first) from the fetch
  const quota = calculateQuota(mergedCount);
  const openCount = openPRs.length;

  // Log history audit
  logger.log('📜 History Audit:');
  if (mergedCount === 0) {
    logger.log('  No merged PRs found.');
  } else if (mergedCount >= 3) {
    logger.log(`  User has ${mergedCount}+ merged PRs (unlimited quota).`);
  } else {
    logger.log(`  User has ${mergedCount} merged PR${mergedCount > 1 ? 's' : ''}.`);
  }

  // Log current stats
  logger.log(`\n📊 Current Stats:`);
  logger.log(`  User has ${mergedCount} merged PRs. Current Quota: ${quota}. Currently Open: ${openCount}.`);

  // Ensure label exists
  if (!dryRun) {
    await ensureLabelExists(octokit, owner, repo, logger);
  }

  // Process each open PR
  const results = {
    blocked: [],
    unblocked: [],
    unchanged: []
  };

  logger.log(`\n🔄 Processing Open PRs:\n`);

  for (let i = 0; i < openPRs.length; i++) {
    const pr = openPRs[i];
    const shouldBeBlocked = i >= quota;
    const isCurrentlyBlocked = pr.labels.some(label => label.name === LABEL_NAME);

    if (shouldBeBlocked && !isCurrentlyBlocked) {
      // Need to block this PR
      if (dryRun) {
        logger.log(`  🔍 [DRY RUN] Would label PR #${pr.number} as blocked (Position: ${i + 1}/${openCount}, Quota: ${quota})`);
        logger.log(`  🔍 [DRY RUN] Would post blocking comment on PR #${pr.number}`);
      } else {
        await addLabel(octokit, owner, repo, pr.number, logger);
        await postBlockingComment(octokit, owner, repo, pr.number, author, openCount, quota, logger);
        logger.log(`  ✅ Labeled PR #${pr.number} as blocked (Position: ${i + 1}/${openCount}, Quota: ${quota})`);
      }
      results.blocked.push(pr.number);
    } else if (!shouldBeBlocked && isCurrentlyBlocked) {
      // Need to unblock this PR
      if (dryRun) {
        logger.log(`  🔍 [DRY RUN] Would remove label from PR #${pr.number} (Position: ${i + 1}/${openCount}, Quota: ${quota})`);
        logger.log(`  🔍 [DRY RUN] Would post unblocking comment on PR #${pr.number}`);
      } else {
        await removeLabel(octokit, owner, repo, pr.number, logger);
        await postUnblockingComment(octokit, owner, repo, pr.number, author, openCount, quota, logger);
        logger.log(`  ✅ Unblocked PR #${pr.number} (Position: ${i + 1}/${openCount}, Quota: ${quota})`);
      }
      results.unblocked.push(pr.number);
    } else {
      results.unchanged.push(pr.number);
      logger.log(`  ℹ️  PR #${pr.number} unchanged (${shouldBeBlocked ? 'blocked' : 'active'})`);
    }
  }

  logger.log(`\n✅ Processing Complete for @${author}\n`);

  return {
    author,
    mergedCount,
    quota,
    openCount,
    results
  };
}

/**
 * Ensure the pr-quota-reached label exists in the repository
 */
async function ensureLabelExists(octokit, owner, repo, logger) {
  try {
    await octokit.rest.issues.getLabel({
      owner,
      repo,
      name: LABEL_NAME
    });
  } catch (error) {
    if (error.status === 404) {
      logger.log(`🏷️  Creating label: ${LABEL_NAME}`);
      await octokit.rest.issues.createLabel({
        owner,
        repo,
        name: LABEL_NAME,
        color: LABEL_COLOR,
        description: 'PR is on hold due to quota limits for new contributors'
      });
    } else {
      throw error;
    }
  }
}

/**
 * Add the quota-reached label to a PR
 */
async function addLabel(octokit, owner, repo, issueNumber, logger) {
  try {
    await octokit.rest.issues.addLabels({
      owner,
      repo,
      issue_number: issueNumber,
      labels: [LABEL_NAME]
    });
  } catch (error) {
    // Fail the job: a quota that cannot be applied is a quota that does not exist.
    throw new Error(`Failed to add label to PR #${issueNumber}: ${error.message}`, { cause: error });
  }
}

/**
 * Remove the quota-reached label from a PR
 */
async function removeLabel(octokit, owner, repo, issueNumber, logger) {
  try {
    await octokit.rest.issues.removeLabel({
      owner,
      repo,
      issue_number: issueNumber,
      name: LABEL_NAME
    });
  } catch (error) {
    // A 404 means the label was not present, which is already the desired state.
    if (error.status !== 404) {
      throw new Error(`Failed to remove label from PR #${issueNumber}: ${error.message}`, { cause: error });
    }
  }
}

/**
 * Check if a blocking comment already exists on the PR
 */
async function hasBlockingComment(octokit, owner, repo, issueNumber) {
  const { data: comments } = await octokit.rest.issues.listComments({
    owner,
    repo,
    issue_number: issueNumber
  });

  return comments.some(comment =>
    comment.body && comment.body.includes('This PR is currently **on hold**')
  );
}



/**
 * Post a blocking comment to a PR
 */
async function postBlockingComment(octokit, owner, repo, issueNumber, author, openCount, quota, logger) {
  // Check if blocking comment already exists
  if (await hasBlockingComment(octokit, owner, repo, issueNumber)) {
    logger.log(`  ℹ️  Blocking comment already exists on PR #${issueNumber}, skipping.`);
    return;
  }

  const message = `Hi @${author}, thanks for your contribution! To ensure quality reviews, we limit how many concurrent PRs new contributors can open:
${formatStatus(openCount, quota)}

This PR is currently **on hold**. We will automatically move this into the review queue once your existing PRs are merged or closed.

Please see our [Contributing Guidelines](https://github.com/jaegertracing/jaeger/blob/main/CONTRIBUTING_GUIDELINES.md#pull-request-limits-for-new-contributors) for details on our tiered quota policy.`;

  try {
    await octokit.rest.issues.createComment({
      owner,
      repo,
      issue_number: issueNumber,
      body: message
    });
  } catch (error) {
    throw new Error(`Failed to post blocking comment on PR #${issueNumber}: ${error.message}`, { cause: error });
  }
}

/**
 * Post an unblocking comment to a PR
 * Always posts when called - if PR was blocked again after being unblocked, user should be notified again
 */
async function postUnblockingComment(octokit, owner, repo, issueNumber, author, openCount, quota, logger) {
  const message = `PR quota unlocked!

@${author}, this PR has been moved out of the waiting room and into the active review queue:
${formatStatus(openCount, quota)}

Thank you for your patience.`;

  try {
    await octokit.rest.issues.createComment({
      owner,
      repo,
      issue_number: issueNumber,
      body: message
    });
  } catch (error) {
    throw new Error(`Failed to post unblocking comment on PR #${issueNumber}: ${error.message}`, { cause: error });
  }
}

function labelNames(labels) {
  return labels.nodes.map(label => label.name);
}

function isActivePR(pr) {
  return pr.state === 'OPEN' && !labelNames(pr.labels).includes(STALE_LABEL_NAME);
}

function primaryPR(pullRequests) {
  return [...pullRequests].sort((left, right) =>
    left.createdAt.localeCompare(right.createdAt) || left.number - right.number
  )[0];
}

function issueLimitComment(duplicates) {
  const references = duplicates
    .sort((left, right) => left.issueNumber - right.issueNumber)
    .map(({ issueNumber, primaryNumber }) => `- Issue #${issueNumber}: PR #${primaryNumber} is the primary pull request.`)
    .join('\n');
  return `${ISSUE_LIMIT_COMMENT_MARKER}
This pull request is marked as a duplicate because another active pull request is already linked to the same issue.

${references}

If these are intentionally competing implementations or an umbrella issue, a maintainer can apply the \`${OVERRIDE_LABEL_NAME}\` label to the issue.`;
}

async function listIssueLimitComments(octokit, owner, repo, issueNumber) {
  const comments = [];
  for (let page = 1; ; page++) {
    const { data } = await octokit.rest.issues.listComments({
      owner,
      repo,
      issue_number: issueNumber,
      per_page: 100,
      page
    });
    comments.push(...data.filter(comment => comment.body?.includes(ISSUE_LIMIT_COMMENT_MARKER)));
    if (data.length < 100) return comments;
  }
}

async function addIssueLabel(octokit, owner, repo, issueNumber, label) {
  try {
    await octokit.rest.issues.addLabels({ owner, repo, issue_number: issueNumber, labels: [label] });
  } catch (error) {
    throw new Error(`Failed to add ${label} label to PR #${issueNumber}: ${error.message}`, { cause: error });
  }
}

async function removeIssueLabel(octokit, owner, repo, issueNumber, label) {
  try {
    await octokit.rest.issues.removeLabel({ owner, repo, issue_number: issueNumber, name: label });
  } catch (error) {
    if (error.status !== 404) {
      throw new Error(`Failed to remove ${label} label from PR #${issueNumber}: ${error.message}`, { cause: error });
    }
  }
}

async function ensureIssueLimitLabels(octokit, owner, repo) {
  const labels = [
    [DUPLICATE_LABEL_NAME, 'CFD3D7', 'Pull request duplicates another active pull request for an issue'],
    [OVERRIDE_LABEL_NAME, '0E8A16', 'Allow multiple active pull requests for this issue']
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

async function fetchLinkedIssues(octokit, owner, repo, pullRequestNumber) {
  const issues = [];
  let after = null;
  do {
    const result = await octokit.graphql(linkedIssuesQuery, { owner, repo, number: pullRequestNumber, after });
    const connection = result.repository.pullRequest?.closingIssuesReferences;
    if (!connection) return issues;
    issues.push(...connection.nodes);
    after = connection.pageInfo?.hasNextPage ? connection.pageInfo.endCursor : null;
  } while (after);
  return issues;
}

async function fetchLinkedPullRequests(octokit, owner, repo, issueNumber) {
  const pullRequests = [];
  let after = null;
  let issue;
  do {
    const result = await octokit.graphql(linkedPullRequestsQuery, { owner, repo, number: issueNumber, after });
    issue = result.repository.issue;
    const connection = issue.closedByPullRequestsReferences;
    pullRequests.push(...connection.nodes);
    after = connection.pageInfo?.hasNextPage ? connection.pageInfo.endCursor : null;
  } while (after);
  return { issue, pullRequests };
}

/**
 * Reconcile the per-issue active PR limit for every same-repository issue
 * canonically linked to a pull request. Reads complete before any mutation so
 * an API failure cannot remove an existing duplicate indication.
 */
async function processIssueLimitForPullRequest(octokit, owner, repo, pullRequestNumber, logger = console, dryRun = false) {
  const repository = `${owner}/${repo}`;
  const linkedIssues = (await fetchLinkedIssues(octokit, owner, repo, pullRequestNumber))
    .filter(issue => issue.repository.nameWithOwner === repository);

  const reconciliations = await Promise.all(linkedIssues.map(async ({ number }) => {
    const { issue, pullRequests } = await fetchLinkedPullRequests(octokit, owner, repo, number);
    return {
      number,
      overridden: labelNames(issue.labels).includes(OVERRIDE_LABEL_NAME),
      pullRequests: pullRequests
        .filter(pr => pr.repository.nameWithOwner === repository && isActivePR(pr))
    };
  }));

  const desiredDuplicates = new Map();
  for (const issue of reconciliations) {
    if (issue.overridden || issue.pullRequests.length <= 1) continue;
    const primary = primaryPR(issue.pullRequests);
    for (const pr of issue.pullRequests) {
      if (pr.number === primary.number) continue;
      const duplicates = desiredDuplicates.get(pr.number) || [];
      duplicates.push({ issueNumber: issue.number, primaryNumber: primary.number });
      desiredDuplicates.set(pr.number, duplicates);
    }
  }

  const knownPRs = new Map();
  for (const issue of reconciliations) {
    for (const pr of issue.pullRequests) knownPRs.set(pr.number, pr);
  }
  // A PR whose links were edited away still needs its previous bot marker removed.
  if (!knownPRs.has(pullRequestNumber)) {
    const { data } = await octokit.rest.issues.get({ owner, repo, issue_number: pullRequestNumber });
    knownPRs.set(pullRequestNumber, {
      number: pullRequestNumber,
      labels: { nodes: data.labels.map(label => ({ name: label.name })) }
    });
  }

  const commentSets = new Map();
  await Promise.all([...knownPRs.keys()].map(async number => {
    commentSets.set(number, await listIssueLimitComments(octokit, owner, repo, number));
  }));

  if (dryRun) return {
    reconciledIssues: reconciliations.map(issue => issue.number).sort((left, right) => left - right),
    duplicates: [...desiredDuplicates.keys()].sort((left, right) => left - right)
  };

  await ensureIssueLimitLabels(octokit, owner, repo);
  // Add/update first. A later API failure must not remove an existing signal.
  for (const [number, duplicates] of desiredDuplicates) {
    const pr = knownPRs.get(number);
    if (!labelNames(pr.labels).includes(DUPLICATE_LABEL_NAME)) {
      await addIssueLabel(octokit, owner, repo, number, DUPLICATE_LABEL_NAME);
    }
    const body = issueLimitComment(duplicates);
    const comments = commentSets.get(number);
    if (comments.length === 0) {
      await octokit.rest.issues.createComment({ owner, repo, issue_number: number, body });
    } else if (comments[0].body !== body) {
      await octokit.rest.issues.updateComment({ owner, repo, comment_id: comments[0].id, body });
    }
  }
  // Remove only labels paired with our marker, never a label applied by a human.
  for (const [number, pr] of knownPRs) {
    if (desiredDuplicates.has(number)) continue;
    const comments = commentSets.get(number);
    if (comments.length === 0) continue;
    if (labelNames(pr.labels).includes(DUPLICATE_LABEL_NAME)) {
      await removeIssueLabel(octokit, owner, repo, number, DUPLICATE_LABEL_NAME);
    }
    for (const comment of comments) {
      await octokit.rest.issues.deleteComment({ owner, repo, comment_id: comment.id });
    }
  }
  // Collapse any historical duplicate bot comments after the desired state is durable.
  for (const [number, comments] of commentSets) {
    if (!desiredDuplicates.has(number)) continue;
    for (const comment of comments.slice(1)) {
      await octokit.rest.issues.deleteComment({ owner, repo, comment_id: comment.id });
    }
  }

  return {
    reconciledIssues: reconciliations.map(issue => issue.number).sort((left, right) => left - right),
    duplicates: [...desiredDuplicates.keys()].sort((left, right) => left - right)
  };
}

/**
 * Main execution function for manual CLI usage
 */
async function main() {
  const args = process.argv.slice(2);

  if (args.length < 1) {
    console.error('Usage: GITHUB_TOKEN=<token> node pr-quota-manager.js <username> [owner] [repo]');
    process.exit(1);
  }

  const username = args[0];
  const owner = args[1] || process.env.GITHUB_REPOSITORY?.split('/')[0] || 'jaegertracing';
  const repo = args[2] || process.env.GITHUB_REPOSITORY?.split('/')[1] || 'jaeger';
  const dryRun = process.env.DRY_RUN === 'true' || args.includes('--dry-run');

  if (!process.env.GITHUB_TOKEN) {
    console.error('Error: GITHUB_TOKEN environment variable is required');
    process.exit(1);
  }

  // Import @octokit/rest dynamically for CLI usage
  const { Octokit } = await import('@octokit/rest');
  const octokit = new Octokit({
    auth: process.env.GITHUB_TOKEN
  });

  try {
    const result = await processQuotaForAuthor(octokit, owner, repo, username, console, dryRun);
    console.log('\n📋 Summary:');
    console.log(`  - Blocked: ${result.results.blocked.length} PRs`);
    console.log(`  - Unblocked: ${result.results.unblocked.length} PRs`);
    console.log(`  - Unchanged: ${result.results.unchanged.length} PRs`);
  } catch (error) {
    console.error('Error:', error.message);
    process.exit(1);
  }
}

// GitHub Actions wrapper function
async function githubActionHandler({github, core, username, owner, repo, pullRequestNumber, dryRun = false}) {
  if (!owner || !repo) {
    core.setFailed('Owner and repo are required');
    return;
  }

  try {
    if (username) {
      const result = await processQuotaForAuthor(github, owner, repo, username, console, dryRun);
      core.info('');
      core.info('=== Author quota summary ===');
      core.info(`Blocked: ${result.results.blocked.length} PRs`);
      core.info(`Unblocked: ${result.results.unblocked.length} PRs`);
      core.info(`Unchanged: ${result.results.unchanged.length} PRs`);
    }
    if (pullRequestNumber) {
      const result = await processIssueLimitForPullRequest(
        github, owner, repo, Number(pullRequestNumber), console, dryRun
      );
      core.info(`Reconciled linked issues: ${result.reconciledIssues.join(', ') || 'none'}`);
      core.info(`Duplicate PRs: ${result.duplicates.join(', ') || 'none'}`);
    }
    if (!username && !pullRequestNumber) {
      core.setFailed('A username or pull request number is required');
    }
  } catch (error) {
    core.setFailed(`Error processing quota: ${error.message}`);
    throw error;
  }
}

// Export for GitHub Actions usage
if (typeof module !== 'undefined' && module.exports) {
  // Default export is the GitHub Actions handler
  module.exports = githubActionHandler;

  // Named exports for testing and direct usage
  module.exports.formatStatus = formatStatus;
  module.exports.calculateQuota = calculateQuota;
  module.exports.fetchAuthorPRs = fetchAuthorPRs;
  module.exports.processQuotaForAuthor = processQuotaForAuthor;
  module.exports.ensureLabelExists = ensureLabelExists;
  module.exports.addLabel = addLabel;
  module.exports.removeLabel = removeLabel;
  module.exports.hasBlockingComment = hasBlockingComment;
  module.exports.postBlockingComment = postBlockingComment;
  module.exports.postUnblockingComment = postUnblockingComment;
  module.exports.isActivePR = isActivePR;
  module.exports.primaryPR = primaryPR;
  module.exports.issueLimitComment = issueLimitComment;
  module.exports.processIssueLimitForPullRequest = processIssueLimitForPullRequest;
  module.exports.DUPLICATE_LABEL_NAME = DUPLICATE_LABEL_NAME;
  module.exports.OVERRIDE_LABEL_NAME = OVERRIDE_LABEL_NAME;
  module.exports.ISSUE_LIMIT_COMMENT_MARKER = ISSUE_LIMIT_COMMENT_MARKER;
  module.exports.LABEL_NAME = LABEL_NAME;
  module.exports.LABEL_COLOR = LABEL_COLOR;
}

// Run main function if executed directly
if (require.main === module) {
  main().catch(error => {
    console.error('Fatal error:', error);
    process.exit(1);
  });
}
