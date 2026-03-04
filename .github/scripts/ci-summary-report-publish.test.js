// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

'use strict';

const {
  safeNum,
  computeMetrics,
  computeCoverage,
  buildCommentBody,
  postCheckRun,
  postOrUpdateComment,
  COMMENT_TAG,
} = require('./ci-summary-report-publish');

// ── safeNum ──────────────────────────────────────────────────────────────────

describe('safeNum', () => {
  test('returns null for null', () => expect(safeNum(null)).toBeNull());
  test('returns null for undefined', () => expect(safeNum(undefined)).toBeNull());
  test('returns 0 for 0', () => expect(safeNum(0)).toBe(0));
  test('returns integer value', () => expect(safeNum(5)).toBe(5));
  test('returns float value', () => expect(safeNum(96.8)).toBe(96.8));
  test('returns null for negative number', () => expect(safeNum(-1)).toBeNull());
  test('returns null for NaN', () => expect(safeNum(NaN)).toBeNull());
  test('returns null for Infinity', () => expect(safeNum(Infinity)).toBeNull());
  test('coerces numeric string', () => expect(safeNum('42')).toBe(42));
  test('returns null for non-numeric string', () => expect(safeNum('bad')).toBeNull());
});

// ── computeMetrics ────────────────────────────────────────────────────────────

describe('computeMetrics', () => {
  test('success when no changes and no infra errors', () => {
    const r = computeMetrics({ metrics_has_infra_errors: false, metrics_total_changes: 0 });
    expect(r.conclusion).toBe('success');
    expect(r.text).toBe('✅ No significant metric changes');
    expect(r.totalChanges).toBe(0);
    expect(r.hasInfraErrors).toBe(false);
  });

  test('failure when total changes > 0', () => {
    const r = computeMetrics({ metrics_has_infra_errors: false, metrics_total_changes: 3 });
    expect(r.conclusion).toBe('failure');
    expect(r.text).toBe('❌ 3 metric change(s) detected');
    expect(r.totalChanges).toBe(3);
  });

  test('failure when infra errors present', () => {
    const r = computeMetrics({ metrics_has_infra_errors: true, metrics_total_changes: 0 });
    expect(r.conclusion).toBe('failure');
    expect(r.text).toBe('❌ Infrastructure error: missing diff artifacts');
    expect(r.hasInfraErrors).toBe(true);
  });

  test('failure when total_changes is null (step did not write output)', () => {
    const r = computeMetrics({ metrics_has_infra_errors: false, metrics_total_changes: null });
    expect(r.conclusion).toBe('failure');
    expect(r.text).toBe('❌ Could not read metrics_total_changes from summary');
    expect(r.totalChanges).toBeNull();
  });

  test('infra errors take precedence over missing total_changes', () => {
    const r = computeMetrics({ metrics_has_infra_errors: true, metrics_total_changes: null });
    expect(r.conclusion).toBe('failure');
    expect(r.text).toBe('❌ Infrastructure error: missing diff artifacts');
  });

  // Ensure JSON string "false" / "true" are not coerced as booleans
  test('treats string "true" for metrics_has_infra_errors as falsy (not === true)', () => {
    const r = computeMetrics({ metrics_has_infra_errors: 'true', metrics_total_changes: 0 });
    expect(r.hasInfraErrors).toBe(false);
  });

  test('treats string "false" for metrics_has_infra_errors as falsy', () => {
    const r = computeMetrics({ metrics_has_infra_errors: 'false', metrics_total_changes: 0 });
    expect(r.hasInfraErrors).toBe(false);
  });
});

// ── computeCoverage ───────────────────────────────────────────────────────────

describe('computeCoverage', () => {
  test('success with pct and baseline', () => {
    const r = computeCoverage({
      coverage_skipped: false,
      coverage_conclusion: 'success',
      coverage_percentage: 96.8,
      coverage_baseline: 46.4,
    });
    expect(r.conclusion).toBe('success');
    expect(r.skipped).toBe(false);
    expect(r.text).toBe('✅ Coverage 96.8% (baseline 46.4%)');
  });

  test('failure when conclusion is failure', () => {
    const r = computeCoverage({
      coverage_skipped: false,
      coverage_conclusion: 'failure',
      coverage_percentage: 94.0,
      coverage_baseline: 96.0,
    });
    expect(r.conclusion).toBe('failure');
    expect(r.text).toBe('❌ Coverage 94% (baseline 96%)');
  });

  test('skipped when coverage_skipped is true', () => {
    const r = computeCoverage({ coverage_skipped: true, coverage_conclusion: 'success' });
    expect(r.skipped).toBe(true);
    expect(r.conclusion).toBe('success');
    expect(r.text).toBe('⏭️ No coverage profiles found; coverage gate skipped.');
  });

  test('skipped when coverage_conclusion is "skipped"', () => {
    const r = computeCoverage({ coverage_skipped: false, coverage_conclusion: 'skipped' });
    expect(r.skipped).toBe(true);
    expect(r.conclusion).toBe('success');
  });

  test('shows "unknown" pct when percentage is null', () => {
    const r = computeCoverage({
      coverage_skipped: false,
      coverage_conclusion: 'failure',
      coverage_percentage: null,
      coverage_baseline: null,
    });
    expect(r.text).toBe('❌ Coverage unknown (no baseline)');
  });

  test('shows "no baseline" when baseline is null but pct is known', () => {
    const r = computeCoverage({
      coverage_skipped: false,
      coverage_conclusion: 'success',
      coverage_percentage: 97.0,
      coverage_baseline: null,
    });
    expect(r.text).toBe('✅ Coverage 97% (no baseline)');
  });

  // Ensure JSON string "false" is not coerced as boolean true
  test('treats string "true" for coverage_skipped as not skipped', () => {
    const r = computeCoverage({
      coverage_skipped: 'true',
      coverage_conclusion: 'success',
      coverage_percentage: 97.0,
      coverage_baseline: 96.0,
    });
    expect(r.skipped).toBe(false);
  });
});

// ── buildCommentBody ──────────────────────────────────────────────────────────

describe('buildCommentBody', () => {
  const metricsText  = '✅ No significant metric changes';
  const coverageText = '✅ Coverage 96.8% (baseline 46.4%)';
  const footer       = '➡️ links\n_2026-03-04 00:00:00 UTC_';

  test('starts with COMMENT_TAG for idempotent find-and-update', () => {
    const body = buildCommentBody(metricsText, coverageText, footer);
    expect(body.startsWith(COMMENT_TAG)).toBe(true);
  });

  test('contains expected section headers', () => {
    const body = buildCommentBody(metricsText, coverageText, footer);
    expect(body).toContain('## CI Summary Report');
    expect(body).toContain('### Metrics Comparison');
    expect(body).toContain('### Code Coverage');
  });

  test('embeds metrics and coverage text', () => {
    const body = buildCommentBody(metricsText, coverageText, footer);
    expect(body).toContain(metricsText);
    expect(body).toContain(coverageText);
  });

  test('ends with footer', () => {
    const body = buildCommentBody(metricsText, coverageText, footer);
    expect(body.endsWith(footer)).toBe(true);
  });
});

// ── postCheckRun ──────────────────────────────────────────────────────────────

describe('postCheckRun', () => {
  const owner = 'org', repo = 'repo', headSha = 'abc123';

  test('calls checks.create with correct parameters and logs result', async () => {
    const mockGithub = {
      rest: { checks: { create: jest.fn().mockResolvedValue({ data: { id: 42, html_url: 'https://example.com/check/42' } }) } },
    };
    const mockCore = { info: jest.fn() };

    await postCheckRun(mockGithub, owner, repo, headSha, 'Coverage Gate', 'success',
      { title: 'Coverage Gate', summary: '✅ ok', text: 'footer' }, mockCore);

    expect(mockGithub.rest.checks.create).toHaveBeenCalledWith({
      owner, repo, head_sha: headSha,
      name: 'Coverage Gate',
      status: 'completed',
      conclusion: 'success',
      output: { title: 'Coverage Gate', summary: '✅ ok', text: 'footer' },
    });
    expect(mockCore.info).toHaveBeenCalledWith(expect.stringContaining('Coverage Gate'));
    expect(mockCore.info).toHaveBeenCalledWith(expect.stringContaining('id=42'));
  });
});

// ── postOrUpdateComment ───────────────────────────────────────────────────────

describe('postOrUpdateComment', () => {
  const owner = 'org', repo = 'repo', prNumber = 99;
  const body  = `${COMMENT_TAG}\n## CI Summary Report`;

  test('creates a new comment when none exists', async () => {
    const mockGithub = {
      paginate: jest.fn().mockResolvedValue([{ id: 1, body: 'unrelated comment' }]),
      rest: { issues: {
        listComments: jest.fn(),
        createComment: jest.fn().mockResolvedValue({ data: { id: 201, html_url: 'https://example.com/comment/201' } }),
      }},
    };
    const mockCore = { info: jest.fn() };

    await postOrUpdateComment(mockGithub, owner, repo, prNumber, body, mockCore);

    expect(mockGithub.rest.issues.createComment).toHaveBeenCalledWith(
      expect.objectContaining({ owner, repo, issue_number: prNumber, body })
    );
    expect(mockCore.info).toHaveBeenCalledWith(expect.stringContaining('id=201'));
  });

  test('updates existing comment when one is found', async () => {
    const existingComment = { id: 100, body: `${COMMENT_TAG}\nold content` };
    const mockGithub = {
      paginate: jest.fn().mockResolvedValue([existingComment]),
      rest: { issues: {
        listComments: jest.fn(),
        updateComment: jest.fn().mockResolvedValue({ data: { html_url: 'https://example.com/comment/100' } }),
      }},
    };
    const mockCore = { info: jest.fn() };

    await postOrUpdateComment(mockGithub, owner, repo, prNumber, body, mockCore);

    expect(mockGithub.rest.issues.updateComment).toHaveBeenCalledWith(
      expect.objectContaining({ owner, repo, comment_id: 100, body })
    );
    expect(mockCore.info).toHaveBeenCalledWith(expect.stringContaining('Updating existing comment id=100'));
  });
});
