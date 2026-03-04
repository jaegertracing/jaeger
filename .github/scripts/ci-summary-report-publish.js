// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// SECURITY WARNING — INJECTION RISK
//
// This script runs in the BASE REPOSITORY context (via workflow_run) with
// pull-requests: write and checks: write permissions. The ci-summary artifact
// it reads was produced by a PR's CI run and may originate from a FORK,
// containing UNTRUSTED content crafted by the PR author.
//
// NEVER interpolate artifact content verbatim into PR comments, check run
// summaries, or any GitHub API call. Doing so allows a malicious PR to inject
// arbitrary Markdown or URLs into the repository's UI.
//
// Required invariants maintained by this file:
//   1. ci-summary.json contains ONLY typed primitives: numbers, booleans, and
//      fixed enum strings ("success"/"failure"/"skipped"). No free-form text.
//   2. All display text (PR comments, check summaries) is constructed entirely
//      from trusted template strings defined in this file.
//   3. Numeric values are coerced through safeNum() which validates with
//      Number.isFinite() and rejects negatives.
//   4. Boolean fields are compared with === true, never coerced with !! which
//      would misinterpret a JSON string "false" as truthy.
//   5. String fields from the artifact are used only in comparisons
//      (=== 'success'), never interpolated into output strings.

'use strict';

// HTML comment tag used to identify the CI summary comment on a PR.
const COMMENT_TAG = '<!-- ci-summary-report -->';

/**
 * Coerce a value from ci-summary.json to a non-negative number, or null.
 * Returns null for null/undefined inputs (preserves "step did not run" signal).
 * Returns null for NaN, Infinity, or negative values.
 * @param {*} v
 * @returns {number|null}
 */
function safeNum(v) {
  if (v === null || v === undefined) return null;
  const n = Number(v);
  return Number.isFinite(n) && n >= 0 ? n : null;
}

/**
 * Derive metrics conclusion and display text from the parsed ci-summary artifact.
 * Uses === true for boolean fields to avoid misinterpreting JSON strings.
 * @param {object} s - Parsed ci-summary.json
 * @returns {{ hasInfraErrors: boolean, totalChanges: number|null, conclusion: string, text: string }}
 */
function computeMetrics(s) {
  const hasInfraErrors = s.metrics_has_infra_errors === true;
  const totalChanges   = safeNum(s.metrics_total_changes);
  // Derive conclusion from the same conditions that drive text so they are always consistent.
  const conclusion     = (hasInfraErrors || totalChanges === null || totalChanges > 0) ? 'failure' : 'success';

  let text;
  if (hasInfraErrors) {
    text = '❌ Infrastructure error: missing diff artifacts';
  } else if (totalChanges === null) {
    text = '❌ Could not read metrics_total_changes from summary';
  } else if (totalChanges > 0) {
    text = `❌ ${totalChanges} metric change(s) detected`;
  } else {
    text = '✅ No significant metric changes';
  }

  return { hasInfraErrors, totalChanges, conclusion, text };
}

/**
 * Derive coverage conclusion and display text from the parsed ci-summary artifact.
 * @param {object} s - Parsed ci-summary.json
 * @returns {{ skipped: boolean, conclusion: string, text: string }}
 */
function computeCoverage(s) {
  const skipped    = s.coverage_skipped === true || s.coverage_conclusion === 'skipped';
  const conclusion = (skipped || s.coverage_conclusion === 'success') ? 'success' : 'failure';
  const pct        = safeNum(s.coverage_percentage);
  const baseline   = safeNum(s.coverage_baseline);

  let text;
  if (skipped) {
    text = '⏭️ No coverage profiles found; coverage gate skipped.';
  } else {
    const pctStr      = pct      !== null ? `${pct}%`      : 'unknown';
    const baselineStr = baseline !== null ? ` (baseline ${baseline}%)` : ' (no baseline)';
    const icon        = conclusion === 'success' ? '✅' : '❌';
    text              = `${icon} Coverage ${pctStr}${baselineStr}`;
  }

  return { skipped, conclusion, text };
}

/**
 * Build the PR comment body from pre-computed display strings.
 * Inputs are strings produced by computeMetrics/computeCoverage: all display text
 * is constructed from trusted templates; artifact-derived values appear only as
 * validated primitives (numbers) embedded by those functions, never as raw strings.
 * @param {string} metricsText
 * @param {string} coverageText
 * @param {string} footer - links + timestamp line
 * @returns {string}
 */
function buildCommentBody(metricsText, coverageText, footer) {
  return [
    COMMENT_TAG,
    '## CI Summary Report',
    '',
    '### Metrics Comparison',
    metricsText,
    '',
    '### Code Coverage',
    coverageText,
    '',
    footer,
  ].join('\n');
}

/**
 * Create a completed check run and log the result.
 * @param {object} github - Octokit client
 * @param {string} owner
 * @param {string} repo
 * @param {string} headSha
 * @param {string} name
 * @param {string} conclusion
 * @param {object} output - { title, summary, text }
 * @param {object} core - GitHub Actions core logger
 */
async function postCheckRun(github, owner, repo, headSha, name, conclusion, output, core) {
  core.info(`Creating check run: "${name}" (conclusion: ${conclusion})`);
  const { data } = await github.rest.checks.create({
    owner, repo,
    head_sha: headSha,
    name,
    status: 'completed',
    conclusion,
    output,
  });
  core.info(`Check run created: id=${data.id} url=${data.html_url}`);
}

/**
 * Post or update the CI summary comment on a PR.
 * Finds an existing comment by COMMENT_TAG and updates it, or creates a new one.
 * @param {object} github - Octokit client
 * @param {string} owner
 * @param {string} repo
 * @param {number} prNumber
 * @param {string} body
 * @param {object} core - GitHub Actions core logger
 */
async function postOrUpdateComment(github, owner, repo, prNumber, body, core) {
  core.info(`Searching for existing CI summary comment on PR #${prNumber}`);
  const existing = await github.paginate(github.rest.issues.listComments, {
    owner, repo, issue_number: prNumber,
  }).then(cs => cs.find(c => c.body && c.body.startsWith(COMMENT_TAG)));

  if (existing) {
    core.info(`Updating existing comment id=${existing.id}`);
    const { data: updated } = await github.rest.issues.updateComment({
      owner, repo, comment_id: existing.id, body,
    });
    core.info(`Comment updated: url=${updated.html_url}`);
  } else {
    core.info(`Creating new comment on PR #${prNumber}`);
    const { data: created } = await github.rest.issues.createComment({
      owner, repo, issue_number: prNumber, body,
    });
    core.info(`Comment created: id=${created.id} url=${created.html_url}`);
  }
}

/**
 * GitHub Actions entry point.
 * Reads ci-summary.json, computes conclusions, posts check runs and PR comment.
 *
 * @param {object} opts
 * @param {object} opts.github  - Octokit client from actions/github-script
 * @param {object} opts.core    - GitHub Actions core logger
 * @param {object} opts.fs      - Node fs module (injected for testability)
 * @param {object} opts.inputs
 * @param {string} opts.inputs.owner
 * @param {string} opts.inputs.repo
 * @param {string} opts.inputs.headSha
 * @param {string} opts.inputs.prNumber  - raw string from step output
 * @param {string} opts.inputs.ciRunUrl
 * @param {string} opts.inputs.publishUrl
 */
async function handler({ github, core, fs, inputs }) {
  const { owner, repo, headSha, ciRunUrl, publishUrl } = inputs;
  const prNumber = parseInt(inputs.prNumber, 10) || null;

  const links  = `➡️ [View CI run](${ciRunUrl}) | [View publish logs](${publishUrl})`;
  const ts     = new Date().toISOString().replace('T', ' ').replace(/\.\d+Z$/, ' UTC');
  const footer = `${links}\n_${ts}_`;

  // Read structured data written by ci-summary-report.yml.
  // All fields are primitives (enums, numbers, booleans) — no free-form text.
  let s;
  try {
    s = JSON.parse(fs.readFileSync('.artifacts/ci-summary.json', 'utf8'));
  } catch (e) {
    core.warning(`ci-summary.json not found or unparseable: ${e.message}`);
    // Post failing check runs so required status checks are never silently absent.
    // All text here is a trusted, fixed string — no artifact content is used.
    const errorSummary = 'ci-summary artifact missing or unparseable; check CI run logs.';
    for (const name of ['Metrics Comparison', 'Coverage Gate']) {
      await postCheckRun(github, owner, repo, headSha, name, 'failure',
        { title: name, summary: errorSummary, text: footer }, core);
    }
    return;
  }

  const metrics  = computeMetrics(s);
  const coverage = computeCoverage(s);

  await postCheckRun(github, owner, repo, headSha, 'Metrics Comparison', metrics.conclusion, {
    title:   'Metrics Comparison Result',
    summary: metrics.text,
    text:    `Total changes across all snapshots: ${metrics.totalChanges ?? 'unknown'}\n\n${footer}`,
  }, core);

  // Always created so it can be used as a required status check.
  await postCheckRun(github, owner, repo, headSha, 'Coverage Gate', coverage.conclusion, {
    title:   'Coverage Gate',
    summary: coverage.text,
    text:    footer,
  }, core);

  // ── PR comment (only when there is something to report) ──────────────────
  const hasIssues = metrics.conclusion === 'failure' || coverage.conclusion === 'failure'
                    || metrics.totalChanges > 0;
  if (!prNumber || !hasIssues) {
    core.info('No issues to report; skipping PR comment.');
    return;
  }

  const body = buildCommentBody(metrics.text, coverage.text, footer);
  await postOrUpdateComment(github, owner, repo, prNumber, body, core);
}

module.exports = handler;
module.exports.safeNum            = safeNum;
module.exports.computeMetrics     = computeMetrics;
module.exports.computeCoverage    = computeCoverage;
module.exports.buildCommentBody   = buildCommentBody;
module.exports.postCheckRun       = postCheckRun;
module.exports.postOrUpdateComment = postOrUpdateComment;
module.exports.COMMENT_TAG        = COMMENT_TAG;
