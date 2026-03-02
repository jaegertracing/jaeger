#!/usr/bin/env node

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
    logger.error(`Failed to add label to PR #${issueNumber}:`, error.message);
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
    // Ignore 404 errors (label wasn't present)
    if (error.status !== 404) {
      logger.error(`Failed to remove label from PR #${issueNumber}:`, error.message);
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
  * Open: ${openCount}
  * Limit: ${quota}

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
    logger.error(`Failed to post blocking comment on PR #${issueNumber}:`, error.message);
  }
}

/**
 * Post an unblocking comment to a PR
 * Always posts when called - if PR was blocked again after being unblocked, user should be notified again
 */
async function postUnblockingComment(octokit, owner, repo, issueNumber, author, openCount, quota, logger) {
  const message = `PR quota unlocked! @${author}, this PR has been moved out of the waiting room and into the active review queue. Thank you for your patience.

**Current Status:** ${openCount}/${quota} open.`;

  try {
    await octokit.rest.issues.createComment({
      owner,
      repo,
      issue_number: issueNumber,
      body: message
    });
  } catch (error) {
    logger.error(`Failed to post unblocking comment on PR #${issueNumber}:`, error.message);
  }
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
async function githubActionHandler({github, core, username, owner, repo, dryRun = false}) {
  if (!username) {
    core.setFailed('Username is required');
    return;
  }
  
  if (!owner || !repo) {
    core.setFailed('Owner and repo are required');
    return;
  }
  
  // Process the quota
  try {
    const result = await processQuotaForAuthor(github, owner, repo, username, console, dryRun);
    
    core.info('');
    core.info('=== Summary ===');
    core.info(`Blocked: ${result.results.blocked.length} PRs`);
    core.info(`Unblocked: ${result.results.unblocked.length} PRs`);
    core.info(`Unchanged: ${result.results.unchanged.length} PRs`);
    
    if (result.results.blocked.length > 0) {
      core.info(`Blocked PRs: ${result.results.blocked.join(', ')}`);
    }
    
    if (result.results.unblocked.length > 0) {
      core.info(`Unblocked PRs: ${result.results.unblocked.join(', ')}`);
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
  module.exports.calculateQuota = calculateQuota;
  module.exports.fetchAuthorPRs = fetchAuthorPRs;
  module.exports.processQuotaForAuthor = processQuotaForAuthor;
  module.exports.ensureLabelExists = ensureLabelExists;
  module.exports.addLabel = addLabel;
  module.exports.removeLabel = removeLabel;
  module.exports.hasBlockingComment = hasBlockingComment;
  module.exports.postBlockingComment = postBlockingComment;
  module.exports.postUnblockingComment = postUnblockingComment;
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
