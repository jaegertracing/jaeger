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
// summaries, log lines, workflow commands, or any GitHub API call. Doing so
// allows a malicious PR to inject arbitrary Markdown, URLs, or a workflow
// command into this privileged job.
//
// The artifact never chooses where results are written. resolvePublishTarget()
// takes the commit and PR from the CI run that triggered this job (or the run
// named by workflow_dispatch), as reported by GitHub.
//
// Required invariants maintained by this file:
//   1. ci-summary.json contains ONLY typed primitives: numbers, booleans, and
//      fixed enum strings ("success"/"failure"/"skipped"). No free-form text.
//      Any other field, including pr_number or head_sha from an older
//      artifact, is ignored.
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

// Prometheus metric names: must start with [a-zA-Z_:], followed by [a-zA-Z0-9_:].
const METRIC_NAME_RE = /^[a-zA-Z_:][a-zA-Z0-9_:]*$/;
const MAX_METRIC_NAME_LEN = 200;
const MAX_SNAPSHOT_NAME_LEN = 200;
const MAX_SNAPSHOTS = 50;
const MAX_METRIC_NAMES_PER_SNAPSHOT = 200;
// Snapshot names come from artifact directory names (alphanumeric, underscores,
// dots, hyphens).  We reject anything outside this character set.
const SNAPSHOT_NAME_RE = /^[a-zA-Z0-9_.\-]+$/;

/**
 * Validate and sanitize a Prometheus metric name.
 * Returns the name if valid, null otherwise.
 * @param {*} name
 * @returns {string|null}
 */
function sanitizeMetricName(name) {
  if (typeof name !== 'string') return null;
  if (name.length === 0 || name.length > MAX_METRIC_NAME_LEN) return null;
  return METRIC_NAME_RE.test(name) ? name : null;
}

/**
 * Validate and sanitize the metrics_snapshots array from ci-summary.json.
 * Each entry is validated: counts go through safeNum(), metric names through
 * sanitizeMetricName(). Invalid entries or fields are silently dropped.
 * @param {*} raw - The raw metrics_snapshots value from the artifact
 * @returns {Array|null} - Sanitized array, or null if input is missing/invalid
 */
function sanitizeSnapshots(raw) {
  if (!Array.isArray(raw)) return null;
  const result = [];
  for (const entry of raw) {
    if (result.length >= MAX_SNAPSHOTS) break;
    if (typeof entry !== 'object' || entry === null) continue;
    // Validate snapshot name
    const snapshot = typeof entry.snapshot === 'string'
      && entry.snapshot.length > 0
      && entry.snapshot.length <= MAX_SNAPSHOT_NAME_LEN
      && SNAPSHOT_NAME_RE.test(entry.snapshot)
      ? entry.snapshot : null;
    if (!snapshot) continue;
    // Validate counts
    const added    = safeNum(entry.added);
    const removed  = safeNum(entry.removed);
    const modified = safeNum(entry.modified);
    // Validate metric_names array — collect up to cap valid names
    const names = [];
    if (Array.isArray(entry.metric_names)) {
      for (const n of entry.metric_names) {
        if (names.length >= MAX_METRIC_NAMES_PER_SNAPSHOT) break;
        const clean = sanitizeMetricName(n);
        if (clean) names.push(clean);
      }
    }
    // Validate artifact_id (optional, non-negative integer for the diff artifact download link)
    const artifactId = safeNum(entry.artifact_id);
    // artifact_id must be a positive integer (GitHub artifact IDs are always > 0)
    const sanitizedArtifactId = (artifactId !== null && Number.isInteger(artifactId) && artifactId > 0)
      ? artifactId : null;
    result.push({ snapshot, added, removed, modified, metric_names: names, artifact_id: sanitizedArtifactId });
  }
  return result.length > 0 ? result : null;
}

/**
 * Derive metrics conclusion and display text from the parsed ci-summary artifact.
 * Uses === true for boolean fields to avoid misinterpreting JSON strings.
 * @param {object} s - Parsed ci-summary.json
 * @returns {{ hasInfraErrors: boolean, totalChanges: number|null, snapshots: Array|null, conclusion: string, text: string }}
 */
function computeMetrics(s) {
  const hasInfraErrors = s.metrics_has_infra_errors === true;
  const totalChanges   = safeNum(s.metrics_total_changes);
  const snapshots      = sanitizeSnapshots(s.metrics_snapshots);
  // Metric diffs are informational on PRs because the snapshot set can include
  // nondeterministic runtime series. Infra/reporting failures still block.
  const conclusion     = (hasInfraErrors || totalChanges === null) ? 'failure' : 'success';

  let text;
  if (hasInfraErrors) {
    text = '❌ Infrastructure error: missing diff artifacts';
  } else if (totalChanges === null) {
    text = '❌ Could not read metrics_total_changes from summary';
  } else if (totalChanges > 0) {
    text = `⚠️ ${totalChanges} metric change(s) detected (informational)`;
  } else {
    text = '✅ No significant metric changes';
  }

  return { hasInfraErrors, totalChanges, snapshots, conclusion, text };
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
 * Format a detail breakdown of per-snapshot metric changes.
 * All text is built from trusted templates; metric names have been validated
 * through sanitizeMetricName() and are rendered in backtick-code spans.
 * @param {Array|null} snapshots - Sanitized snapshots from computeMetrics
 * @param {string} [ciRunUrl] - URL to the CI run for linking to step logs
 * @param {string} [artifactUrlPrefix] - Base URL for artifact downloads:
 *   `https://github.com/{owner}/{repo}/actions/runs/{runId}/artifacts`.
 *   When provided, each snapshot with a valid artifact_id gets a direct
 *   download link appended to its header line.
 * @returns {string} - Markdown detail block, or empty string if no data
 */
function formatMetricsDetail(snapshots, { ciRunUrl, artifactUrlPrefix } = {}) {
  if (!snapshots || snapshots.length === 0) return '';

  const lines = [
    '',
    '<details>',
    '<summary>View changed metrics</summary>',
    '',
  ];

  if (ciRunUrl) {
    lines.push(`_For label-level diff details, open the [CI run](${ciRunUrl}) and expand the "Compare metrics and generate summary" step logs._`);
    lines.push('');
  }

  for (const snap of snapshots) {
    // Build the snapshot header, optionally with a direct diff artifact download link.
    let snapHeader = `**${snap.snapshot}**`;
    if (artifactUrlPrefix && snap.artifact_id !== null && snap.artifact_id !== undefined) {
      snapHeader += ` — [⬇️ download diff](${artifactUrlPrefix}/${snap.artifact_id})`;
    }
    lines.push(snapHeader);
    const parts = [];
    if (snap.added    !== null && snap.added    > 0) parts.push(`${snap.added} added`);
    if (snap.removed  !== null && snap.removed  > 0) parts.push(`${snap.removed} removed`);
    if (snap.modified !== null && snap.modified > 0) parts.push(`${snap.modified} modified`);
    if (parts.length > 0) {
      lines.push(parts.join(', '));
    }
    if (snap.metric_names.length > 0) {
      for (const name of snap.metric_names) {
        lines.push(`- \`${name}\``);
      }
    }
    lines.push('');
  }

  lines.push('</details>');
  return lines.join('\n');
}

/**
 * Build the PR comment body from pre-computed display strings.
 * Inputs are strings produced by computeMetrics/computeCoverage: all display text
 * is constructed from trusted templates; artifact-derived values appear only as
 * validated primitives (numbers) embedded by those functions, never as raw strings.
 * @param {string} metricsText
 * @param {string} coverageText
 * @param {string} footer - links + timestamp line
 * @param {object} [opts]
 * @param {Array|null} [opts.metricsSnapshots] - sanitized snapshot data for detail rendering
 * @param {string} [opts.ciRunUrl] - URL to the CI run, passed through to formatMetricsDetail
 * @param {string} [opts.artifactUrlPrefix] - Base URL for artifact downloads, passed through
 * @returns {string}
 */
function buildCommentBody(metricsText, coverageText, footer, { metricsSnapshots, ciRunUrl, artifactUrlPrefix } = {}) {
  const parts = [
    COMMENT_TAG,
    '## CI Summary Report',
    '',
    '### Metrics Comparison',
    metricsText,
  ];

  const detail = formatMetricsDetail(metricsSnapshots, { ciRunUrl, artifactUrlPrefix });
  if (detail) {
    parts.push(detail);
  }

  parts.push('');
  parts.push('### Code Coverage');
  parts.push(coverageText);
  parts.push('');
  parts.push(footer);

  return parts.join('\n');
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
 * Always updates an existing comment (clears stale failure messages on green runs).
 * Only creates a new comment when createNew is true.
 * @param {object} github - Octokit client
 * @param {string} owner
 * @param {string} repo
 * @param {number} prNumber
 * @param {string} body
 * @param {object} core - GitHub Actions core logger
 * @param {object} [opts]
 * @param {boolean} [opts.createNew=true] - create a comment if none exists
 */
async function postOrUpdateComment(github, owner, repo, prNumber, body, core, { createNew = true } = {}) {
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
  } else if (createNew) {
    core.info(`Creating new comment on PR #${prNumber}`);
    const { data: created } = await github.rest.issues.createComment({
      owner, repo, issue_number: prNumber, body,
    });
    core.info(`Comment created: id=${created.id} url=${created.html_url}`);
  } else {
    core.info('No existing comment and no issues to report; skipping PR comment.');
  }
}


// PR numbers stay inside the range GitHub issue numbers actually use.
const PR_NUMBER_RE = /^[1-9][0-9]{0,8}$/;
// Actions run IDs are wider than PR numbers but must stay within MAX_SAFE_INTEGER.
const RUN_ID_RE = /^[1-9][0-9]{0,14}$/;
const SHA_RE = /^[0-9a-f]{40}$/;
const PR_EVENTS = new Set(['pull_request', 'pull_request_target']);

/**
 * Accept a positive integer from a JSON number or a whole decimal string.
 * Rejects any string parseInt would truncate (newline, suffix, leading zero).
 * @param {*} value
 * @param {RegExp} re
 * @returns {number|null}
 */
function validateBoundedInt(value, re) {
  if (typeof value === 'number') {
    if (!Number.isSafeInteger(value) || value <= 0) return null;
    return re.test(String(value)) ? value : null;
  }
  if (typeof value !== 'string' || !re.test(value)) return null;
  const n = Number(value);
  return Number.isSafeInteger(n) && n > 0 ? n : null;
}

/**
 * @param {*} value
 * @returns {number|null}
 */
function validatePrNumber(value) {
  return validateBoundedInt(value, PR_NUMBER_RE);
}

/**
 * @param {*} value
 * @returns {number|null}
 */
function validateRunId(value) {
  return validateBoundedInt(value, RUN_ID_RE);
}

/**
 * @param {*} value
 * @returns {string|null}
 */
function validateHeadSha(value) {
  return typeof value === 'string' && SHA_RE.test(value) ? value : null;
}

/**
 * Return the number of the one PR in prs, or null when there is not exactly one.
 * @param {Array} prs
 * @returns {number|null}
 */
function singlePrNumber(prs) {
  return prs.length === 1 ? validatePrNumber(prs[0].number) : null;
}

/**
 * @param {object} pr - a PR from the Pulls API
 * @param {string} headSha
 * @returns {boolean} whether the PR is open with headSha as its head
 */
function isOpenAt(pr, headSha) {
  return pr?.state === 'open' && pr?.head?.sha === headSha;
}

/**
 * Find the open PR in this repository whose head is the commit the CI run tested.
 *
 * workflow_run.pull_requests is not limited to this repository: on a push to
 * main it lists PRs in forks whose head is main, numbered in the fork's own
 * sequence. It is also empty for fork PRs. Entries are kept only when they
 * target this repository at the run's head SHA, and a fork PR is found by its
 * head repository and branch, again requiring the same head SHA.
 *
 * @param {object} args
 * @param {object} args.github
 * @param {string} args.owner
 * @param {string} args.repo
 * @param {number} args.repoId - id of the base repository
 * @param {object} args.run - the CI Orchestrator workflow run
 * @returns {Promise<number|null>}
 */
async function findPullRequest({ github, owner, repo, repoId, run }) {
  if (!PR_EVENTS.has(run.event)) return null;

  const listed = (Array.isArray(run.pull_requests) ? run.pull_requests : [])
    .filter(pr => pr?.base?.repo?.id === repoId && pr?.head?.sha === run.head_sha);
  if (listed.length > 0) {
    const number = singlePrNumber(listed);
    if (!number) return null;
    // pull_requests was captured when the CI run finished. Confirm the PR has
    // not been closed or moved on since, as the head-branch lookup below does.
    const { data: live } = await github.rest.pulls.get({ owner, repo, pull_number: number });
    return isOpenAt(live, run.head_sha) ? number : null;
  }

  const headRepo = run.head_repository;
  if (!headRepo?.owner?.login || !run.head_branch) return null;
  const { data } = await github.rest.pulls.list({
    owner, repo,
    head: `${headRepo.owner.login}:${run.head_branch}`,
    state: 'open',
    per_page: 10,
  });
  return singlePrNumber((Array.isArray(data) ? data : [])
    .filter(pr => pr?.head?.repo?.id === headRepo.id && pr?.head?.sha === run.head_sha));
}

/**
 * Choose the CI run, commit, and PR this privileged job writes to.
 *
 * The commit is always the CI run's head_sha. For a pull_request run that is
 * the PR branch tip (the synthetic merge commit is only GITHUB_SHA inside the
 * run), so check runs land on the commit whose results they report, even if
 * the PR has moved on since.
 *
 * workflow_run: the run is the event's workflow_run.
 * workflow_dispatch: the run is fetched by the run_id input. A pr_number input
 * is used only if that PR is still open with the run's head SHA as its head.
 *
 * Nothing here reads the ci-summary artifact. Warnings are fixed strings.
 *
 * @param {object} args
 * @param {object} args.github - Octokit client
 * @param {object} args.context - actions/github-script context
 * @returns {Promise<{runId: number|null, headSha: string|null, prNumber: number|null, warnings: string[]}>}
 */
async function resolvePublishTarget({ github, context }) {
  const warnings = [];
  const none = { runId: null, headSha: null, prNumber: null, warnings };
  const { owner, repo } = context.repo;
  const payload = context.payload || {};

  let run;
  let dispatchPr = null;
  let lookUpPr = true;
  if (context.eventName === 'workflow_run') {
    run = payload.workflow_run;
  } else if (context.eventName === 'workflow_dispatch') {
    const inputs = payload.inputs || {};
    const runId = validateRunId(inputs.run_id);
    if (!runId) {
      warnings.push('workflow_dispatch run_id is not a valid run ID.');
      return none;
    }
    ({ data: run } = await github.rest.actions.getWorkflowRun({ owner, repo, run_id: runId }));
    if (inputs.pr_number) {
      lookUpPr = false;
      dispatchPr = validatePrNumber(inputs.pr_number);
      if (!dispatchPr) warnings.push('workflow_dispatch pr_number is not a valid PR number; skipping the PR comment.');
    }
  } else {
    warnings.push('Unsupported event; skipping publish.');
    return none;
  }

  const runId = validateRunId(run?.id);
  const headSha = validateHeadSha(run?.head_sha);
  if (!runId || !headSha) {
    warnings.push('CI run has no valid run ID or head SHA.');
    return none;
  }

  let prNumber = null;
  try {
    if (dispatchPr) {
      const { data: pr } = await github.rest.pulls.get({ owner, repo, pull_number: dispatchPr });
      if (isOpenAt(pr, headSha)) {
        prNumber = dispatchPr;
      } else {
        warnings.push('workflow_dispatch pr_number is not an open PR with the CI run head SHA as its head; skipping the PR comment.');
      }
    } else if (lookUpPr) {
      prNumber = await findPullRequest({ github, owner, repo, repoId: payload.repository?.id, run });
      if (!prNumber && PR_EVENTS.has(run.event)) {
        warnings.push('No single open PR in this repository has the CI run head SHA as its head; skipping the PR comment.');
      }
    }
  } catch {
    warnings.push('Pulls API request failed; skipping the PR comment.');
  }

  return { runId, headSha, prNumber, warnings };
}

/**
 * @param {object} fs
 * @returns {object|null} the parsed artifact, or null if it is missing or not a JSON object
 */
function readSummaryArtifact(fs) {
  let parsed;
  try {
    parsed = JSON.parse(fs.readFileSync('.artifacts/ci-summary.json', 'utf8'));
  } catch {
    return null;
  }
  return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : null;
}

/**
 * GitHub Actions entry point.
 * Reads ci-summary.json, computes conclusions, posts check runs and PR comment.
 * The run, commit, and PR come from resolvePublishTarget, never the artifact.
 *
 * @param {object} opts
 * @param {object} opts.github  - Octokit client from actions/github-script
 * @param {object} opts.core    - GitHub Actions core logger
 * @param {object} opts.context - actions/github-script context
 * @param {object} opts.fs      - Node fs module (injected for testability)
 */
async function handler({ github, core, context, fs }) {
  const { owner, repo } = context.repo;
  const target = await resolvePublishTarget({ github, context });
  for (const warning of target.warnings) core.warning(warning);
  const { runId, headSha, prNumber } = target;
  if (!headSha) {
    core.warning('No CI run head SHA; skipping check runs and PR comment.');
    return;
  }
  core.info(`Publishing CI run ${runId} for ${headSha}${prNumber ? ` on PR #${prNumber}` : ''}`);

  // owner/repo come from the base repository and runId was validated as an integer.
  const runsUrl    = `https://github.com/${owner}/${repo}/actions/runs`;
  const ciRunUrl   = `${runsUrl}/${runId}`;
  const publishUrl = `${runsUrl}/${context.runId}`;
  const links      = `➡️ [View CI run](${ciRunUrl}) | [View publish logs](${publishUrl})`;
  const ts         = new Date().toISOString().replace('T', ' ').replace(/\.\d+Z$/, ' UTC');
  const footer     = `${links}\n_${ts}_`;
  const artifactUrlPrefix = `${ciRunUrl}/artifacts`;

  // Read structured data written by ci-summary-report.yml.
  // All fields are primitives (enums, numbers, booleans) — no free-form text.
  const s = readSummaryArtifact(fs);
  if (!s) {
    // Fixed text only: JSON.parse error messages quote the file contents.
    core.warning('ci-summary.json not found or unparseable.');
    // Post failing check runs so required status checks are never silently absent.
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

  // ── PR comment ──
  if (prNumber) {
    // Always update an existing comment so stale failure messages don't linger
    // after a green run.  Only create a new comment when there is something to report.
    const hasIssues = metrics.conclusion === 'failure' || coverage.conclusion === 'failure'
                      || metrics.totalChanges > 0;
    const body = buildCommentBody(metrics.text, coverage.text, footer, {
      metricsSnapshots: metrics.snapshots,
      ciRunUrl,
      artifactUrlPrefix,
    });
    await postOrUpdateComment(github, owner, repo, prNumber, body, core, { createNew: hasIssues });
  } else {
    core.info('No PR number; skipping PR comment.');
  }
}

module.exports = handler;
module.exports.safeNum              = safeNum;
module.exports.sanitizeMetricName   = sanitizeMetricName;
module.exports.sanitizeSnapshots    = sanitizeSnapshots;
module.exports.computeMetrics       = computeMetrics;
module.exports.computeCoverage      = computeCoverage;
module.exports.formatMetricsDetail  = formatMetricsDetail;
module.exports.buildCommentBody     = buildCommentBody;
module.exports.postCheckRun         = postCheckRun;
module.exports.postOrUpdateComment  = postOrUpdateComment;
module.exports.COMMENT_TAG          = COMMENT_TAG;
module.exports.validatePrNumber     = validatePrNumber;
module.exports.validateRunId        = validateRunId;
module.exports.validateHeadSha      = validateHeadSha;
module.exports.resolvePublishTarget = resolvePublishTarget;
