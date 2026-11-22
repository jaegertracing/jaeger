#!/usr/bin/env node

// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

/**
 * Apply the linked issue policy to the open pull requests of a repository
 * from a local machine. The workflow flags a pull request only when an event
 * touches it, so this script is how the policy reaches the pull requests that
 * were open before the workflow existed.
 *
 * With --close-expired-pull-requests the script does what the scheduled run
 * of the workflow does: it checks only the flagged pull requests and closes
 * the ones that have been flagged for a week.
 *
 * The script reports what it would do and writes nothing unless --apply is
 * given.
 *
 * Usage:
 *   GITHUB_TOKEN=$(gh auth token) node apply-pr-linked-issue-policy.js [--apply] [--close-expired-pull-requests] [--pr <number>] [owner] [repo]
 */

const { parseArgs } = require('node:util');
const { processLinkedIssuePolicy, parseOptionalNumber } = require('../actions/pr-linked-issue/pr-linked-issue.js');

async function main() {
  const { values, positionals } = parseArgs({
    options: {
      apply: { type: 'boolean', default: false },
      'close-expired-pull-requests': { type: 'boolean', default: false },
      pr: { type: 'string' }
    },
    allowPositionals: true
  });
  const [owner = 'jaegertracing', repo = 'jaeger'] = positionals;

  // An empty --pr, for example from an unset shell variable, must not be
  // read as "every pull request".
  if (values.pr === '') {
    console.error('Error: --pr needs a pull request number');
    process.exit(1);
  }

  if (!process.env.GITHUB_TOKEN) {
    console.error('Error: GITHUB_TOKEN environment variable is required');
    process.exit(1);
  }

  const { Octokit } = await import('@octokit/rest');
  const octokit = new Octokit({ auth: process.env.GITHUB_TOKEN });

  const dryRun = !values.apply;
  console.log(`${dryRun ? 'Dry run of' : 'Applying'} the linked issue policy on ${owner}/${repo}\n`);
  const result = await processLinkedIssuePolicy(octokit, owner, repo, {
    pullRequestNumber: parseOptionalNumber(values.pr, '--pr'),
    closeExpiredPullRequests: values['close-expired-pull-requests'],
    dryRun
  });
  console.log(`\nPRs that violate the policy: ${result.flaggedPullRequests.length}`);
  console.log(`PRs that now meet the policy: ${result.unflaggedPullRequests.length}`);
  console.log(`PRs closed: ${result.closedPullRequests.length}`);
}

main().catch(error => {
  console.error(error);
  process.exit(1);
});
