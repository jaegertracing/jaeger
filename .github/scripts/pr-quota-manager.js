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
const COMMENT_REQUEST_CONCURRENCY = 10;
const UNLIMITED_QUOTA = Number.POSITIVE_INFINITY;
// Merged history is shared by the repositories that enforce this policy.
// The open cap is still applied only in the repository that runs the action.
const PROJECT_REPOS = [
  ['jaegertracing', 'jaeger'],
  ['jaegertracing', 'jaeger-ui']
];

const openPullRequestsQuery = `
  query openPullRequests($owner: String!, $repo: String!, $after: String) {
    repository(owner: $owner, name: $repo) {
      pullRequests(states: OPEN, first: 100, after: $after) {
        nodes {
          number
          createdAt
          labels(first: 20) {
            nodes { name }
            pageInfo { hasNextPage }
          }
          closingIssuesReferences(first: 10) {
            nodes {
              number
              repository { nameWithOwner }
              labels(first: 10) {
                nodes { name }
                pageInfo { hasNextPage }
              }
            }
            pageInfo { hasNextPage }
          }
        }
        pageInfo { hasNextPage endCursor }
      }
    }
  }
`;

/*
 * This policy intentionally uses GitHub's repository-wide `duplicate` label,
 * as requested in #9250. The marker identifies comments owned by this action;
 * it is never treated as an authority signal when posted by another account.
 */
const ISSUE_LIMIT_COMMENT_MARKER = '<!-- jaeger-pr-per-issue-limit -->';
const ISSUE_LIMIT_OWNED_LABEL_MARKER = '<!-- jaeger-pr-per-issue-limit-label-owned -->';

/** Render a finite quota, or the word unlimited when there is no cap. */
function formatQuota(quota) {
  return Number.isFinite(quota) ? String(quota) : 'unlimited';
}

/**
 * Format open/limit counts as a bullet-point status block
 * @param {number} openCount - Number of currently open PRs
 * @param {number} quota - Allowed quota
 * @returns {string} Formatted status string
 */
function formatStatus(openCount, quota) {
  return `  * Open: ${openCount}\n  * Limit: ${formatQuota(quota)}`;
}

/**
 * Logins from QUOTA_EXEMPT_USERS. Commas and whitespace separate entries, and a
 * leading @ is ignored. Computed on each check so tests can change the variable.
 * @returns {string[]} Lowercased logins
 */
function exemptNames() {
  return (process.env.QUOTA_EXEMPT_USERS || '')
    .split(/[\s,]+/)
    .map(name => name.replace(/^@/, '').trim().toLowerCase())
    .filter(Boolean);
}

/**
 * Case-insensitive exemption list from the QUOTA_EXEMPT_USERS repository variable.
 * @param {string} author - PR author username
 * @returns {boolean} Whether the author skips the quota
 */
function isExempt(author) {
  const login = String(author || '').replace(/^@/, '').trim().toLowerCase();
  return login !== '' && exemptNames().includes(login);
}

/**
 * Calculate the quota for a user based on their merged PR count.
 * Three or more merged PRs is unlimited: no open PR is labeled.
 * @param {number} mergedCount - Number of merged PRs across the project repositories
 * @returns {number} The allowed quota, or Infinity when there is no cap
 */
function calculateQuota(mergedCount) {
  if (mergedCount === 0) return 1;
  if (mergedCount === 1) return 2;
  if (mergedCount === 2) return 3;
  return UNLIMITED_QUOTA;
}

function quoteSearchTerm(value) {
  return `"${String(value).replace(/"/g, '')}"`;
}

/**
 * Search qualifier for the pull request opener.
 * The Copilot account's login is Copilot, and author:"Copilot" is rejected
 * with 422. GitHub's app qualifier is the one that matches those pull requests.
 * @param {string} author - PR author username
 * @returns {string} author: qualifier, including the author: prefix
 */
function authorSearchQualifier(author) {
  if (String(author || '').toLowerCase() === 'copilot') {
    return 'author:app/copilot-swe-agent';
  }
  return `author:${quoteSearchTerm(author)}`;
}

function isPullRequestEvent(eventName) {
  return eventName === 'pull_request' || eventName === 'pull_request_target';
}

/**
 * 422 from Search when the login does not exist, was deleted, or cannot be
 * queried in author:"..." form.
 * @param {Error} error - Octokit or test error
 * @returns {boolean}
 */
function isUnsearchableAuthorError(error) {
  const status = error && (error.status || error.response?.status);
  const details = [
    error && error.message,
    error && error.response?.data?.message,
    ...((error && error.response?.data?.errors) || []).map(item => item && item.message)
  ].filter(Boolean).join(' ');
  return status === 422 && /cannot be searched/i.test(details);
}

function sameRepository(item, owner, repo) {
  const expected = `${owner}/${repo}`.toLowerCase();
  const url = String(item && item.repository_url || '').toLowerCase();
  if (url.endsWith(`/repos/${expected}`)) return true;
  const fullName = item && item.repository && (item.repository.full_name || item.repository.nameWithOwner);
  return String(fullName || '').toLowerCase() === expected;
}

/**
 * A closed-label search hit may be removed only when it is this repository and,
 * for an author sweep, that author. Missing identity fields are not a match.
 * @param {object} item - Search result
 * @param {string} owner - Repository owner
 * @param {string} repo - Repository name
 * @param {string} [author] - Required login, or omitted for the repo-wide sweep
 * @returns {boolean}
 */
function closedHitMatches(item, owner, repo, author) {
  if (!item || !item.number || !sameRepository(item, owner, repo)) return false;
  if (!author) return true;
  const login = item.user && item.user.login;
  return Boolean(login) && login.toLowerCase() === String(author).toLowerCase();
}

/**
 * Run a GitHub issue search. A failed or incomplete search throws so the job
 * fails instead of falling back to a single-repository count.
 * @param {object} octokit - GitHub API client
 * @param {string} q - Search query
 * @param {{paginate?: boolean, perPage?: number}} [options] - Page size. Pagination uses 100.
 * @returns {Promise<{totalCount: number, items: Array}>} Search total and items
 */
async function searchIssues(octokit, q, options = {}) {
  const paginate = options.paginate === true;
  const items = [];
  let totalCount = 0;
  const maxPage = paginate ? 10 : 1;
  const pageSize = paginate ? 100 : (options.perPage || 1);

  for (let page = 1; page <= maxPage; page++) {
    const { data } = await octokit.rest.search.issuesAndPullRequests({
      q,
      per_page: pageSize,
      page
    });

    if (data.incomplete_results) {
      throw new Error(`GitHub search returned incomplete results for query: ${q}`);
    }
    if (typeof data.total_count !== 'number') {
      throw new Error(`GitHub search returned no total_count for query: ${q}`);
    }

    totalCount = data.total_count;
    const pageItems = data.items || [];
    items.push(...pageItems);

    if (!paginate || items.length >= totalCount || pageItems.length < 100) {
      break;
    }
  }

  if (paginate && items.length < totalCount) {
    throw new Error(`GitHub search returned ${items.length} of ${totalCount} results for query: ${q}`);
  }

  return { totalCount, items };
}

/**
 * Count pull requests the author opened that were merged in jaeger or jaeger-ui.
 * Search matches the pull request opener, so a squash merge still counts.
 * @param {object} octokit - GitHub API client
 * @param {string} author - PR author username
 * @returns {Promise<number>} Merged pull request count
 */
/**
 * If this run is the merge of a pull request Search has not indexed yet, count it.
 * total_count below 3 cannot already include that pull request when per_page is 3
 * and none of the returned items is the event pull request.
 * @param {number} totalCount - Search total_count
 * @param {Array} items - First page of merged results
 * @param {object} [eventPullRequest] - Closed event, with owner and repo set
 * @returns {number}
 */
function mergedCountWithEvent(totalCount, items, eventPullRequest) {
  if (!eventPullRequest || eventPullRequest.action !== 'closed' || eventPullRequest.merged !== true) {
    return totalCount;
  }
  const owner = eventPullRequest.owner;
  const repo = eventPullRequest.repo;
  const inProject = PROJECT_REPOS.some(([projectOwner, projectRepo]) =>
    projectOwner.toLowerCase() === String(owner || '').toLowerCase() &&
    projectRepo.toLowerCase() === String(repo || '').toLowerCase()
  );
  if (!inProject) return totalCount;
  const number = Number(eventPullRequest.number);
  const seen = (items || []).some(item =>
    Number(item.number) === number && sameRepository(item, owner, repo)
  );
  if (!seen && totalCount < 3) return totalCount + 1;
  return totalCount;
}

async function countProjectMergedPRs(octokit, author, eventPullRequest) {
  const repos = PROJECT_REPOS.map(([owner, repo]) => `repo:${owner}/${repo}`).join(' ');
  const q = `is:pr is:merged ${authorSearchQualifier(author)} ${repos}`;
  const { totalCount, items } = await searchIssues(octokit, q, { perPage: 3 });
  return mergedCountWithEvent(totalCount, items, eventPullRequest);
}

/**
 * Closed pull requests in this repository that still wear the quota label.
 * @param {object} octokit - GitHub API client
 * @param {string} owner - Repository owner
 * @param {string} repo - Repository name
 * @param {string} author - PR author username
 * @returns {Promise<Array>} Closed pull requests to clear
 */
async function listClosedQuotaPRs(octokit, owner, repo, author) {
  const q = [
    `repo:${owner}/${repo}`,
    'is:pr',
    'is:closed',
    `label:${quoteSearchTerm(LABEL_NAME)}`,
    authorSearchQualifier(author)
  ].join(' ');
  const { items } = await searchIssues(octokit, q, { paginate: true });
  return items;
}

/**
 * Every closed pull request in this repository that still has the quota label.
 * @param {object} octokit - GitHub API client
 * @param {string} owner - Repository owner
 * @param {string} repo - Repository name
 * @returns {Promise<Array>} Search hits
 */
async function listAllClosedQuotaPRs(octokit, owner, repo) {
  const q = [
    `repo:${owner}/${repo}`,
    'is:pr',
    'is:closed',
    `label:${quoteSearchTerm(LABEL_NAME)}`
  ].join(' ');
  const { items } = await searchIssues(octokit, q, { paginate: true });
  return items;
}

/**
 * Open pull requests in this repository. Pass an author to keep only that login.
 * The comparison is case-insensitive. A missing user is skipped.
 * @param {object} octokit - GitHub API client
 * @param {string} owner - Repository owner
 * @param {string} repo - Repository name
 * @param {string} [author] - Login to keep, or omitted for every open pull request
 * @returns {Promise<Array>} Open pull requests, oldest first
 */
async function listOpenPullRequests(octokit, owner, repo, author) {
  const openPRs = [];
  const perPage = 100;
  const authorKey = author ? String(author).toLowerCase() : '';
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
    const pagePRs = authorKey
      ? data.filter(pr => pr.user && String(pr.user.login || '').toLowerCase() === authorKey)
      : data;
    openPRs.push(...pagePRs);
    if (data.length < perPage) break;
    page++;
  }
  return openPRs;
}

/**
 * Fetch this repository's open PRs by the author, the shared merged-PR count,
 * and closed PRs in this repository that still have the quota label.
 * Both searches run before any label write. A search error rejects the promise.
 * @param {object} octokit - GitHub API client
 * @param {string} owner - Repository owner/org
 * @param {string} repo - Repository name
 * @param {string} author - PR author username
 * @param {object} [eventPullRequest] - Closed merge event used to correct index lag
 * @returns {Promise<{openPRs: Array, mergedCount: number, closedLabeledPRs: Array}>}
 */
async function fetchAuthorPRs(octokit, owner, repo, author, eventPullRequest) {
  const openPRs = await listOpenPullRequests(octokit, owner, repo, author);
  const mergedCount = await countProjectMergedPRs(octokit, author, eventPullRequest && {
    ...eventPullRequest,
    owner,
    repo
  });
  const closedLabeledPRs = await listClosedQuotaPRs(octokit, owner, repo, author);

  return {
    openPRs,
    mergedCount,
    closedLabeledPRs
  };
}

/**
 * Numbers whose quota label should be removed because they are closed.
 * Search hits that are still in the REST open list are left for the open-PR
 * loop: the index can still call a reopened PR closed.
 * The event pull request is included on close even when Search has not indexed it.
 * @param {Array} closedLabeledPRs - Search hits
 * @param {Array} openPRs - REST open pull requests for this author
 * @param {string} owner - Repository owner
 * @param {string} repo - Repository name
 * @param {string} author - PR author username
 * @param {object} [eventPullRequest] - Triggering pull request, if any
 * @returns {number[]}
 */
function closedNumbersToClear(closedLabeledPRs, openPRs, owner, repo, author, eventPullRequest) {
  const openNumbers = new Set(openPRs.map(pr => Number(pr.number)));
  const numbers = [];
  function add(number) {
    if (!number || openNumbers.has(number) || numbers.includes(number)) return;
    numbers.push(number);
  }
  for (const pr of closedLabeledPRs) {
    if (closedHitMatches(pr, owner, repo, author)) add(Number(pr.number));
  }
  if (eventPullRequest && eventPullRequest.action === 'closed') {
    add(Number(eventPullRequest.number));
  }
  return numbers;
}

async function clearClosedNumbers(octokit, owner, repo, numbers, logger, dryRun) {
  for (const number of numbers) {
    if (dryRun) {
      logger.log(`  🔍 [DRY RUN] Would remove label from closed PR #${number}`);
    } else {
      await removeLabel(octokit, owner, repo, number, logger);
      logger.log(`  ✅ Removed label from closed PR #${number}`);
    }
  }
  return numbers;
}

/**
 * Process quota management for a specific author
 * @param {object} octokit - GitHub API client
 * @param {string} owner - Repository owner
 * @param {string} repo - Repository name
 * @param {string} author - PR author username
 * @param {object} logger - Logger object (console or custom)
 * @param {boolean} dryRun - If true, only print actions without executing them
 * @param {{eventName?: string, eventPullRequest?: object}} [options] - Trigger context
 * @returns {Promise<object>} Processing results
 */
async function processQuotaForAuthor(octokit, owner, repo, author, logger = console, dryRun = false, options = {}) {
  const exempt = isExempt(author);
  const eventPullRequest = options.eventPullRequest;
  const eventName = options.eventName;
  if (dryRun) {
    logger.log('🔍 DRY RUN MODE - No changes will be made\n');
  }
  logger.log(`\n=== Processing Quota for: @${author} ===\n`);

  // Open pull requests come from REST. Search runs only for authors who are
  // not exempt, and only after that decision, so an exempt login makes no
  // Search calls.
  const openPRs = await listOpenPullRequests(octokit, owner, repo, author);

  let mergedCount = 0;
  let closedLabeledPRs = [];
  if (!exempt) {
    const eventForCount = eventPullRequest ? { ...eventPullRequest, owner, repo } : undefined;
    try {
      mergedCount = await countProjectMergedPRs(octokit, author, eventForCount);
      closedLabeledPRs = await listClosedQuotaPRs(octokit, owner, repo, author);
    } catch (error) {
      if (isUnsearchableAuthorError(error) && isPullRequestEvent(eventName)) {
        logger.log(`  Search cannot look up @${author}. Skipping quota changes.`);
        const closedUnlabeled = await clearClosedNumbers(
          octokit, owner, repo,
          closedNumbersToClear([], openPRs, owner, repo, author, eventPullRequest),
          logger, dryRun
        );
        return {
          author,
          mergedCount: 0,
          quota: null,
          openCount: openPRs.length,
          skipped: true,
          results: { blocked: [], unblocked: [], unchanged: [], closedUnlabeled }
        };
      }
      throw error;
    }
  }

  // Open PRs are already sorted by creation date (oldest first) from the fetch.
  // An exempt login is unlimited regardless of merged history, which is not queried.
  const quota = exempt ? UNLIMITED_QUOTA : calculateQuota(mergedCount);
  const openCount = openPRs.length;
  const quotaText = formatQuota(quota);

  // Log history audit
  logger.log('📜 History Audit:');
  if (exempt) {
    logger.log('  User is exempt from the PR quota.');
  } else if (mergedCount === 0) {
    logger.log('  No merged PRs found.');
  } else if (!Number.isFinite(quota)) {
    logger.log(`  User has ${mergedCount} merged PR${mergedCount === 1 ? '' : 's'} in the project (unlimited quota).`);
  } else {
    logger.log(`  User has ${mergedCount} merged PR${mergedCount > 1 ? 's' : ''}.`);
  }

  // Log current stats
  logger.log(`\n📊 Current Stats:`);
  if (exempt) {
    logger.log(`  User is exempt. Current Quota: unlimited. Currently Open: ${openCount}.`);
  } else {
    logger.log(`  User has ${mergedCount} merged PRs. Current Quota: ${quotaText}. Currently Open: ${openCount}.`);
  }

  // Ensure label exists
  if (!dryRun) {
    await ensureLabelExists(octokit, owner, repo, logger);
  }

  // Process each open PR
  const results = {
    blocked: [],
    unblocked: [],
    unchanged: [],
    closedUnlabeled: []
  };

  logger.log(`\n🔄 Processing Open PRs:\n`);

  for (let i = 0; i < openPRs.length; i++) {
    const pr = openPRs[i];
    const shouldBeBlocked = i >= quota;
    const isCurrentlyBlocked = pr.labels.some(label => label.name === LABEL_NAME);

    if (shouldBeBlocked && !isCurrentlyBlocked) {
      // Need to block this PR
      if (dryRun) {
        logger.log(`  🔍 [DRY RUN] Would label PR #${pr.number} as blocked (Position: ${i + 1}/${openCount}, Quota: ${quotaText})`);
        logger.log(`  🔍 [DRY RUN] Would post blocking comment on PR #${pr.number}`);
      } else {
        await addLabel(octokit, owner, repo, pr.number, logger);
        await postBlockingComment(octokit, owner, repo, pr.number, author, openCount, quota, logger);
        logger.log(`  ✅ Labeled PR #${pr.number} as blocked (Position: ${i + 1}/${openCount}, Quota: ${quotaText})`);
      }
      results.blocked.push(pr.number);
    } else if (!shouldBeBlocked && isCurrentlyBlocked) {
      // Need to unblock this PR
      if (dryRun) {
        logger.log(`  🔍 [DRY RUN] Would remove label from PR #${pr.number} (Position: ${i + 1}/${openCount}, Quota: ${quotaText})`);
        logger.log(`  🔍 [DRY RUN] Would post unblocking comment on PR #${pr.number}`);
      } else {
        await removeLabel(octokit, owner, repo, pr.number, logger);
        await postUnblockingComment(octokit, owner, repo, pr.number, author, openCount, quota, logger);
        logger.log(`  ✅ Unblocked PR #${pr.number} (Position: ${i + 1}/${openCount}, Quota: ${quotaText})`);
      }
      results.unblocked.push(pr.number);
    } else {
      results.unchanged.push(pr.number);
      logger.log(`  ℹ️  PR #${pr.number} unchanged (${shouldBeBlocked ? 'blocked' : 'active'})`);
    }
  }

  // Search can lag REST in both directions. Never clear a number REST still
  // lists as open, and always clear the pull request that just closed.
  // Clearing a closed pull request does not post a comment.
  results.closedUnlabeled = await clearClosedNumbers(
    octokit, owner, repo,
    closedNumbersToClear(closedLabeledPRs, openPRs, owner, repo, author, eventPullRequest),
    logger, dryRun
  );

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
  return (!pr.state || pr.state === 'OPEN') && !labelNames(pr.labels).includes(STALE_LABEL_NAME);
}

function primaryPR(pullRequests) {
  return [...pullRequests].sort((left, right) =>
    left.createdAt.localeCompare(right.createdAt) || left.number - right.number
  )[0];
}

function issueLimitComment(duplicates, ownsDuplicateLabel = true) {
  const references = duplicates
    .sort((left, right) => left.issueNumber - right.issueNumber)
    .map(({ issueNumber, primaryNumber }) => `- Issue #${issueNumber}: PR #${primaryNumber} is the primary pull request.`)
    .join('\n');
  const ownership = ownsDuplicateLabel ? `\n${ISSUE_LIMIT_OWNED_LABEL_MARKER}` : '';
  return `${ISSUE_LIMIT_COMMENT_MARKER}${ownership}
This pull request is marked as a duplicate because another active pull request is already linked to the same issue.

${references}

If these are intentionally competing implementations or an umbrella issue, a maintainer can apply the \`${OVERRIDE_LABEL_NAME}\` label to the issue.`;
}

function issueLimitResolvedComment() {
  return `${ISSUE_LIMIT_COMMENT_MARKER}
This pull request is no longer marked as a duplicate by the active-PR policy.`;
}

const supersededIssueLimitComment =
  'This automated active-PR policy comment has been superseded by the current policy comment.';

function commentOwnsDuplicateLabel(comment) {
  return comment.body?.includes(ISSUE_LIMIT_OWNED_LABEL_MARKER);
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
    comments.push(...data.filter(comment =>
      comment.user?.login === 'github-actions[bot]' && comment.body?.includes(ISSUE_LIMIT_COMMENT_MARKER)
    ));
    if (data.length < 100) return comments;
  }
}

async function forEachWithConcurrency(items, concurrency, callback) {
  let nextIndex = 0;
  async function worker() {
    while (nextIndex < items.length) {
      const item = items[nextIndex++];
      await callback(item);
    }
  }
  await Promise.all(Array.from({ length: Math.min(concurrency, items.length) }, worker));
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

async function fetchOpenPullRequests(octokit, owner, repo) {
  const pullRequests = [];
  let after = null;
  do {
    const result = await octokit.graphql(openPullRequestsQuery, { owner, repo, after });
    const connection = result.repository.pullRequests;
    for (const pr of connection.nodes) {
      if (pr.labels.pageInfo?.hasNextPage) {
        throw new Error(`PR #${pr.number} has more labels than the per-issue policy query supports`);
      }
      if (pr.closingIssuesReferences.pageInfo?.hasNextPage) {
        throw new Error(`PR #${pr.number} has more linked issues than the per-issue policy query supports`);
      }
      for (const issue of pr.closingIssuesReferences.nodes) {
        if (issue.labels.pageInfo?.hasNextPage) {
          throw new Error(`Issue #${issue.number} has more labels than the per-issue policy query supports`);
        }
      }
    }
    pullRequests.push(...connection.nodes);
    after = connection.pageInfo?.hasNextPage ? connection.pageInfo.endCursor : null;
  } while (after);
  return pullRequests;
}

function calculateIssueLimitState(pullRequests, repository) {
  const issues = new Map();
  for (const pr of pullRequests.filter(isActivePR)) {
    for (const issue of pr.closingIssuesReferences.nodes) {
      if (issue.repository.nameWithOwner !== repository) continue;
      const state = issues.get(issue.number) || {
        number: issue.number,
        overridden: labelNames(issue.labels).includes(OVERRIDE_LABEL_NAME),
        pullRequests: []
      };
      state.pullRequests.push(pr);
      issues.set(issue.number, state);
    }
  }

  const desiredDuplicates = new Map();
  for (const issue of issues.values()) {
    if (issue.overridden || issue.pullRequests.length <= 1) continue;
    const primary = primaryPR(issue.pullRequests);
    for (const pr of issue.pullRequests) {
      if (pr.number === primary.number) continue;
      const duplicates = desiredDuplicates.get(pr.number) || [];
      duplicates.push({ issueNumber: issue.number, primaryNumber: primary.number });
      desiredDuplicates.set(pr.number, duplicates);
    }
  }
  return { issues, desiredDuplicates };
}

function linkedIssueNumbers(pr, repository) {
  return new Set(pr.closingIssuesReferences.nodes
    .filter(issue => issue.repository.nameWithOwner === repository)
    .map(issue => issue.number));
}

function issueLimitCommentCandidates(pullRequests, repository, pullRequestNumber, issueNumber) {
  if (!pullRequestNumber && !issueNumber) {
    return new Set(pullRequests.map(pr => pr.number));
  }

  const affectedIssues = new Set(issueNumber ? [Number(issueNumber)] : []);
  const triggeringPR = pullRequests.find(pr => pr.number === Number(pullRequestNumber));
  if (triggeringPR) {
    for (const number of linkedIssueNumbers(triggeringPR, repository)) {
      affectedIssues.add(number);
    }
  }

  return new Set(pullRequests
    .filter(pr =>
      pr.number === Number(pullRequestNumber) ||
      labelNames(pr.labels).includes(DUPLICATE_LABEL_NAME) ||
      [...linkedIssueNumbers(pr, repository)].some(number => affectedIssues.has(number))
    )
    .map(pr => pr.number));
}

function reconciledIssueNumbers(issues, commentCandidates) {
  return [...issues.values()]
    .filter(issue => issue.pullRequests.some(pr => commentCandidates.has(pr.number)))
    .map(issue => issue.number)
    .sort((left, right) => left - right);
}

/**
 * Reconcile the complete open-PR graph in one paginated GraphQL traversal.
 * Computing every PR's links together avoids one issue clearing another issue's
 * duplicate state when a PR is linked to more than one issue.
 */
async function processIssueLimits(
  octokit, owner, repo, logger = console, dryRun = false, pullRequestNumber, issueNumber
) {
  const repository = `${owner}/${repo}`;
  const pullRequests = await fetchOpenPullRequests(octokit, owner, repo);
  const { issues, desiredDuplicates } = calculateIssueLimitState(pullRequests, repository);
  const knownPRs = new Map(pullRequests.map(pr => [pr.number, pr]));
  const commentCandidates = issueLimitCommentCandidates(
    pullRequests, repository, pullRequestNumber, issueNumber
  );
  for (const number of desiredDuplicates.keys()) {
    commentCandidates.add(number);
  }
  const reconciledIssues = reconciledIssueNumbers(issues, commentCandidates);

  const commentSets = new Map();
  // Scheduled/manual sweeps inspect every marker for recovery. Event-driven
  // reconciliation also includes calculated duplicates so the surviving run
  // applies a missed label without exhausting the shared REST API budget.
  await forEachWithConcurrency([...commentCandidates], COMMENT_REQUEST_CONCURRENCY, async number => {
    commentSets.set(number, await listIssueLimitComments(octokit, owner, repo, number));
  });

  if (dryRun) return {
    reconciledIssues,
    duplicates: [...desiredDuplicates.keys()].sort((left, right) => left - right)
  };

  const missingDuplicateLabel = [...desiredDuplicates.keys()].some(number =>
    !labelNames(knownPRs.get(number).labels).includes(DUPLICATE_LABEL_NAME)
  );
  if (missingDuplicateLabel) await ensureIssueLimitLabels(octokit, owner, repo);
  // Add/update first. A later API failure must not remove an existing signal.
  for (const [number, duplicates] of desiredDuplicates) {
    const pr = knownPRs.get(number);
    const comments = commentSets.get(number) || [];
    const hasDuplicateLabel = labelNames(pr.labels).includes(DUPLICATE_LABEL_NAME);
    // Persist ownership before adding a label. If the label write fails, the
    // marker remains discoverable on the next pass and can be resolved safely.
    const ownsDuplicateLabel = comments.some(commentOwnsDuplicateLabel) || !hasDuplicateLabel;
    const body = issueLimitComment(duplicates, ownsDuplicateLabel);
    if (comments.length === 0) {
      await octokit.rest.issues.createComment({ owner, repo, issue_number: number, body });
    } else if (comments[0].body !== body) {
      await octokit.rest.issues.updateComment({ owner, repo, comment_id: comments[0].id, body });
    }
    if (!hasDuplicateLabel) {
      await addIssueLabel(octokit, owner, repo, number, DUPLICATE_LABEL_NAME);
    }
  }
  const cleanup = [];
  for (const [number, pr] of knownPRs) {
    // Cleanup must not update stale PRs: stale policy treats bot activity as
    // renewed activity and could reactivate a PR that should stay inactive.
    if (!commentCandidates.has(number) || !isActivePR(pr) || desiredDuplicates.has(number)) continue;
    const comments = commentSets.get(number) || [];
    if (comments.length === 0) continue;
    cleanup.push({
      number,
      comments,
      hasDuplicateLabel: labelNames(pr.labels).includes(DUPLICATE_LABEL_NAME),
      ownsDuplicateLabel: comments.some(commentOwnsDuplicateLabel)
    });
  }

  const removedLabels = [];
  try {
    // Keep markers intact until every destructive label removal succeeds. If a
    // removal fails, rollback leaves the next run able to identify ownership.
    for (const item of cleanup) {
      if (!item.hasDuplicateLabel || !item.ownsDuplicateLabel) continue;
      await removeIssueLabel(octokit, owner, repo, item.number, DUPLICATE_LABEL_NAME);
      removedLabels.push(item.number);
    }
  } catch (error) {
    await Promise.allSettled(removedLabels.map(number =>
      addIssueLabel(octokit, owner, repo, number, DUPLICATE_LABEL_NAME)
    ));
    throw error;
  }

  for (const item of cleanup) {
    const resolvedBody = issueLimitResolvedComment();
    if (item.comments[0].body !== resolvedBody) {
      await octokit.rest.issues.updateComment({
        owner,
        repo,
        comment_id: item.comments[0].id,
        body: resolvedBody
      });
    }
    for (const comment of item.comments.slice(1)) {
      await octokit.rest.issues.updateComment({
        owner,
        repo,
        comment_id: comment.id,
        body: supersededIssueLimitComment
      });
    }
  }
  // Collapse historical marker comments without deleting user-visible history.
  for (const [number, comments] of commentSets) {
    if (!desiredDuplicates.has(number)) continue;
    for (const comment of comments.slice(1)) {
      await octokit.rest.issues.updateComment({
        owner,
        repo,
        comment_id: comment.id,
        body: supersededIssueLimitComment
      });
    }
  }

  return {
    reconciledIssues,
    duplicates: [...desiredDuplicates.keys()].sort((left, right) => left - right)
  };
}

async function processIssueLimitForPullRequest(octokit, owner, repo, pullRequestNumber, logger = console, dryRun = false) {
  return processIssueLimits(octokit, owner, repo, logger, dryRun, pullRequestNumber);
}

async function processIssueLimitForIssue(octokit, owner, repo, issueNumber, logger = console, dryRun = false) {
  return processIssueLimits(octokit, owner, repo, logger, dryRun, undefined, issueNumber);
}

async function processAllIssueLimits(octokit, owner, repo, logger = console, dryRun = false) {
  return processIssueLimits(octokit, owner, repo, logger, dryRun);
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
    console.log(`  - Closed unlabeled: ${result.results.closedUnlabeled.length} PRs`);
  } catch (error) {
    console.error('Error:', error.message);
    process.exit(1);
  }
}

/**
 * Remove pr-quota-reached from every closed pull request in this repository.
 * Search hits that REST still lists as open are skipped. This does not comment.
 * @param {object} octokit - GitHub API client
 * @param {string} owner - Repository owner
 * @param {string} repo - Repository name
 * @param {object} logger - Logger
 * @param {boolean} dryRun - Log without writing
 * @returns {Promise<number[]>} Pull request numbers cleared or planned
 */
async function sweepClosedQuotaLabels(octokit, owner, repo, logger = console, dryRun = true) {
  if (dryRun) {
    logger.log('🔍 DRY RUN MODE - No changes will be made\n');
  }
  const openPRs = await listOpenPullRequests(octokit, owner, repo);
  const openNumbers = new Set(openPRs.map(pr => Number(pr.number)));
  const hits = await listAllClosedQuotaPRs(octokit, owner, repo);
  const numbers = [];
  for (const hit of hits) {
    const number = Number(hit.number);
    if (!closedHitMatches(hit, owner, repo)) continue;
    if (openNumbers.has(number)) {
      logger.log(`  Not removing the quota label from PR #${number}; REST still lists it as open.`);
      continue;
    }
    numbers.push(number);
  }
  return clearClosedNumbers(octokit, owner, repo, numbers, logger, dryRun);
}

function logIssueLimitResult(core, result, dryRun) {
  const prefix = dryRun ? 'Dry-run plan' : 'Reconciled';
  core.info(`${prefix} linked issues: ${result.reconciledIssues.join(', ') || 'none'}`);
  core.info(`${dryRun ? 'Dry-run plan duplicate' : 'Duplicate'} PRs: ${result.duplicates.join(', ') || 'none'}`);
}

// GitHub Actions wrapper function
function sweepModeOf(value) {
  if (value === 'apply') return 'apply';
  if (value === 'dry-run' || value === 'true') return 'dry-run';
  return 'off';
}

async function githubActionHandler({
  github, core, username, owner, repo, pullRequestNumber, issueNumber,
  perIssueLimit = false, dryRun = false, eventName = '', eventPullRequest = null,
  sweepClosedLabels = 'off'
}) {
  if (!owner || !repo) {
    core.setFailed('Owner and repo are required');
    return;
  }

  const sweepMode = sweepModeOf(sweepClosedLabels);
  try {
    if (sweepMode !== 'off') {
      if (eventName !== 'workflow_dispatch') {
        core.setFailed('The closed-label sweep is only available from workflow_dispatch.');
        return;
      }
      // dry-run is the default. apply writes only when the caller also left dryRun false.
      const sweepDryRun = sweepMode !== 'apply' || dryRun;
      const cleared = await sweepClosedQuotaLabels(github, owner, repo, console, sweepDryRun);
      core.info(`${sweepDryRun ? 'Dry-run would clear' : 'Cleared'} closed quota labels: ${cleared.join(', ') || 'none'}`);
    }
    if (username && !perIssueLimit) {
      const result = await processQuotaForAuthor(
        github, owner, repo, username, console, dryRun, { eventName, eventPullRequest }
      );
      core.info('');
      core.info('=== Author quota summary ===');
      core.info(`Blocked: ${result.results.blocked.length} PRs`);
      core.info(`Unblocked: ${result.results.unblocked.length} PRs`);
      core.info(`Unchanged: ${result.results.unchanged.length} PRs`);
      core.info(`Closed unlabeled: ${result.results.closedUnlabeled.length} PRs`);
    }
    if (pullRequestNumber) {
      const result = await processIssueLimitForPullRequest(
        github, owner, repo, Number(pullRequestNumber), console, dryRun
      );
      logIssueLimitResult(core, result, dryRun);
    }
    if (perIssueLimit && !pullRequestNumber) {
      const result = issueNumber
        ? await processIssueLimitForIssue(github, owner, repo, Number(issueNumber), console, dryRun)
        : await processAllIssueLimits(github, owner, repo, console, dryRun);
      logIssueLimitResult(core, result, dryRun);
    }
    // The per-issue workflow does not run the author quota. A pull request that
    // just closed still loses pr-quota-reached from the event payload.
    if (perIssueLimit && eventPullRequest && eventPullRequest.action === 'closed') {
      const cleared = await clearClosedNumbers(
        github, owner, repo,
        closedNumbersToClear([], [], owner, repo, eventPullRequest.login, eventPullRequest),
        console, dryRun
      );
      core.info(`Closed event unlabeled: ${cleared.join(', ') || 'none'}`);
    }
    if (!username && !pullRequestNumber && !perIssueLimit && sweepMode === 'off') {
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
  module.exports.formatQuota = formatQuota;
  module.exports.isExempt = isExempt;
  module.exports.calculateQuota = calculateQuota;
  module.exports.countProjectMergedPRs = countProjectMergedPRs;
  module.exports.fetchAuthorPRs = fetchAuthorPRs;
  module.exports.authorSearchQualifier = authorSearchQualifier;
  module.exports.closedNumbersToClear = closedNumbersToClear;
  module.exports.sweepClosedQuotaLabels = sweepClosedQuotaLabels;
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
  module.exports.issueLimitResolvedComment = issueLimitResolvedComment;
  module.exports.commentOwnsDuplicateLabel = commentOwnsDuplicateLabel;
  module.exports.fetchOpenPullRequests = fetchOpenPullRequests;
  module.exports.calculateIssueLimitState = calculateIssueLimitState;
  module.exports.issueLimitCommentCandidates = issueLimitCommentCandidates;
  module.exports.reconciledIssueNumbers = reconciledIssueNumbers;
  module.exports.listIssueLimitComments = listIssueLimitComments;
  module.exports.forEachWithConcurrency = forEachWithConcurrency;
  module.exports.processIssueLimits = processIssueLimits;
  module.exports.processIssueLimitForPullRequest = processIssueLimitForPullRequest;
  module.exports.processIssueLimitForIssue = processIssueLimitForIssue;
  module.exports.processAllIssueLimits = processAllIssueLimits;
  module.exports.DUPLICATE_LABEL_NAME = DUPLICATE_LABEL_NAME;
  module.exports.OVERRIDE_LABEL_NAME = OVERRIDE_LABEL_NAME;
  module.exports.ISSUE_LIMIT_COMMENT_MARKER = ISSUE_LIMIT_COMMENT_MARKER;
  module.exports.ISSUE_LIMIT_OWNED_LABEL_MARKER = ISSUE_LIMIT_OWNED_LABEL_MARKER;
  module.exports.LABEL_NAME = LABEL_NAME;
  module.exports.LABEL_COLOR = LABEL_COLOR;
  module.exports.UNLIMITED_QUOTA = UNLIMITED_QUOTA;
  module.exports.PROJECT_REPOS = PROJECT_REPOS;
  module.exports.COMMENT_REQUEST_CONCURRENCY = COMMENT_REQUEST_CONCURRENCY;
}

// Run main function if executed directly
if (require.main === module) {
  main().catch(error => {
    console.error('Fatal error:', error);
    process.exit(1);
  });
}
