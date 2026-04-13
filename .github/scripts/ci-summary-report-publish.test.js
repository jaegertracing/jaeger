// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

'use strict';

const {
  safeNum,
  sanitizeMetricName,
  sanitizeSnapshots,
  computeMetrics,
  computeCoverage,
  formatMetricsDetail,
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

// ── sanitizeMetricName ───────────────────────────────────────────────────────

describe('sanitizeMetricName', () => {
  test('accepts valid Prometheus metric name', () => {
    expect(sanitizeMetricName('http_server_duration')).toBe('http_server_duration');
  });
  test('accepts name with colons', () => {
    expect(sanitizeMetricName('rpc:server_duration:total')).toBe('rpc:server_duration:total');
  });
  test('accepts name starting with underscore', () => {
    expect(sanitizeMetricName('_internal_metric')).toBe('_internal_metric');
  });
  test('rejects empty string', () => {
    expect(sanitizeMetricName('')).toBeNull();
  });
  test('rejects name starting with digit', () => {
    expect(sanitizeMetricName('0invalid')).toBeNull();
  });
  test('rejects name with spaces', () => {
    expect(sanitizeMetricName('metric name')).toBeNull();
  });
  test('rejects markdown injection', () => {
    expect(sanitizeMetricName('[click me](http://evil.com)')).toBeNull();
  });
  test('rejects HTML injection', () => {
    expect(sanitizeMetricName('<img src=x onerror=alert(1)>')).toBeNull();
  });
  test('rejects name with curly braces', () => {
    expect(sanitizeMetricName('metric{label="value"}')).toBeNull();
  });
  test('rejects non-string types', () => {
    expect(sanitizeMetricName(42)).toBeNull();
    expect(sanitizeMetricName(null)).toBeNull();
    expect(sanitizeMetricName(undefined)).toBeNull();
    expect(sanitizeMetricName({})).toBeNull();
  });
  test('rejects names exceeding 200 characters', () => {
    expect(sanitizeMetricName('a'.repeat(201))).toBeNull();
  });
  test('accepts names at exactly 200 characters', () => {
    const name = 'a'.repeat(200);
    expect(sanitizeMetricName(name)).toBe(name);
  });
});

// ── sanitizeSnapshots ────────────────────────────────────────────────────────

describe('sanitizeSnapshots', () => {
  test('returns null for non-array input', () => {
    expect(sanitizeSnapshots(null)).toBeNull();
    expect(sanitizeSnapshots(undefined)).toBeNull();
    expect(sanitizeSnapshots('string')).toBeNull();
    expect(sanitizeSnapshots({})).toBeNull();
  });

  test('returns null for empty array', () => {
    expect(sanitizeSnapshots([])).toBeNull();
  });

  test('sanitizes valid snapshot entry', () => {
    const input = [{
      snapshot: 'metrics_snapshot_cassandra',
      added: 2, removed: 1, modified: 0,
      metric_names: ['http_server_duration', 'rpc_client_duration'],
    }];
    const result = sanitizeSnapshots(input);
    expect(result).toHaveLength(1);
    expect(result[0].snapshot).toBe('metrics_snapshot_cassandra');
    expect(result[0].added).toBe(2);
    expect(result[0].removed).toBe(1);
    expect(result[0].modified).toBe(0);
    expect(result[0].metric_names).toEqual(['http_server_duration', 'rpc_client_duration']);
  });

  test('drops entries with invalid snapshot names', () => {
    const input = [
      { snapshot: 'valid_name', added: 1, removed: 0, modified: 0, metric_names: [] },
      { snapshot: '<script>alert(1)</script>', added: 1, removed: 0, modified: 0, metric_names: [] },
      { snapshot: 'also.valid-name.2', added: 0, removed: 1, modified: 0, metric_names: [] },
    ];
    const result = sanitizeSnapshots(input);
    expect(result).toHaveLength(2);
    expect(result[0].snapshot).toBe('valid_name');
    expect(result[1].snapshot).toBe('also.valid-name.2');
  });

  test('drops invalid metric names from metric_names array', () => {
    const input = [{
      snapshot: 'test_snapshot',
      added: 2, removed: 0, modified: 0,
      metric_names: ['valid_metric', '<injected>', 'another_valid'],
    }];
    const result = sanitizeSnapshots(input);
    expect(result[0].metric_names).toEqual(['valid_metric', 'another_valid']);
  });

  test('caps snapshots at 50 entries', () => {
    const input = Array.from({ length: 60 }, (_, i) => ({
      snapshot: `snap_${i}`,
      added: 1, removed: 0, modified: 0,
      metric_names: [],
    }));
    const result = sanitizeSnapshots(input);
    expect(result).toHaveLength(50);
  });

  test('caps metric_names at 200 per snapshot', () => {
    const names = Array.from({ length: 210 }, (_, i) => `metric_${i}`);
    const input = [{
      snapshot: 'test_snapshot',
      added: 210, removed: 0, modified: 0,
      metric_names: names,
    }];
    const result = sanitizeSnapshots(input);
    expect(result[0].metric_names).toHaveLength(200);
  });

  test('handles missing metric_names gracefully', () => {
    const input = [{
      snapshot: 'test_snapshot',
      added: 1, removed: 0, modified: 0,
    }];
    const result = sanitizeSnapshots(input);
    expect(result[0].metric_names).toEqual([]);
  });

  test('validates counts through safeNum', () => {
    const input = [{
      snapshot: 'test_snapshot',
      added: -1, removed: 'bad', modified: Infinity,
      metric_names: ['valid_metric'],
    }];
    const result = sanitizeSnapshots(input);
    expect(result[0].added).toBeNull();
    expect(result[0].removed).toBeNull();
    expect(result[0].modified).toBeNull();
  });

  test('skips null and non-object entries', () => {
    const input = [null, 42, 'string', { snapshot: 'valid', added: 0, removed: 0, modified: 0, metric_names: [] }];
    const result = sanitizeSnapshots(input);
    expect(result).toHaveLength(1);
    expect(result[0].snapshot).toBe('valid');
  });

  test('sanitizes artifact_id as a positive integer', () => {
    const input = [{
      snapshot: 'test_snapshot',
      added: 1, removed: 0, modified: 0,
      metric_names: ['http_server_duration'],
      artifact_id: 6359406281,
    }];
    const result = sanitizeSnapshots(input);
    expect(result[0].artifact_id).toBe(6359406281);
  });

  test('rejects non-integer artifact_id', () => {
    const input = [{
      snapshot: 'test_snapshot',
      added: 1, removed: 0, modified: 0,
      metric_names: [],
      artifact_id: 'evil_string',
    }];
    const result = sanitizeSnapshots(input);
    expect(result[0].artifact_id).toBeNull();
  });

  test('rejects zero and negative artifact_id', () => {
    const input = [
      { snapshot: 'snap_a', added: 0, removed: 0, modified: 0, metric_names: [], artifact_id: 0 },
      { snapshot: 'snap_b', added: 0, removed: 0, modified: 0, metric_names: [], artifact_id: -1 },
    ];
    const result = sanitizeSnapshots(input);
    expect(result[0].artifact_id).toBeNull();
    expect(result[1].artifact_id).toBeNull();
  });

  test('sets artifact_id to null when absent', () => {
    const input = [{ snapshot: 'test_snapshot', added: 1, removed: 0, modified: 0, metric_names: [] }];
    const result = sanitizeSnapshots(input);
    expect(result[0].artifact_id).toBeNull();
  });
});

// ── computeMetrics ────────────────────────────────────────────────────────────

describe('computeMetrics', () => {
  test('success when no changes and no infra errors', () => {
    const r = computeMetrics({ metrics_has_infra_errors: false, metrics_total_changes: 0 });
    expect(r.conclusion).toBe('success');
    expect(r.text).toBe('✅ No significant metric changes');
    expect(r.totalChanges).toBe(0);
    expect(r.hasInfraErrors).toBe(false);
    expect(r.snapshots).toBeNull();
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

  test('includes sanitized snapshots when present', () => {
    const r = computeMetrics({
      metrics_has_infra_errors: false,
      metrics_total_changes: 2,
      metrics_snapshots: [{
        snapshot: 'cassandra_v2',
        added: 1, removed: 1, modified: 0,
        metric_names: ['http_server_duration', 'rpc_client_duration'],
      }],
    });
    expect(r.snapshots).toHaveLength(1);
    expect(r.snapshots[0].snapshot).toBe('cassandra_v2');
    expect(r.snapshots[0].metric_names).toEqual(['http_server_duration', 'rpc_client_duration']);
  });

  test('returns null snapshots when metrics_snapshots is absent', () => {
    const r = computeMetrics({ metrics_has_infra_errors: false, metrics_total_changes: 0 });
    expect(r.snapshots).toBeNull();
  });
});

// ── formatMetricsDetail ──────────────────────────────────────────────────────

describe('formatMetricsDetail', () => {
  test('returns empty string for null snapshots', () => {
    expect(formatMetricsDetail(null)).toBe('');
  });

  test('returns empty string for empty array', () => {
    expect(formatMetricsDetail([])).toBe('');
  });

  test('renders single snapshot with metric names', () => {
    const detail = formatMetricsDetail([{
      snapshot: 'cassandra_v2',
      added: 1, removed: 0, modified: 1,
      metric_names: ['http_server_duration', 'rpc_client_duration'],
    }]);
    expect(detail).toContain('<details>');
    expect(detail).toContain('</details>');
    expect(detail).toContain('View changed metrics');
    expect(detail).toContain('**cassandra_v2**');
    expect(detail).toContain('1 added, 1 modified');
    expect(detail).toContain('- `http_server_duration`');
    expect(detail).toContain('- `rpc_client_duration`');
  });

  test('renders multiple snapshots', () => {
    const detail = formatMetricsDetail([
      { snapshot: 'snap_a', added: 2, removed: 0, modified: 0, metric_names: ['metric_a'] },
      { snapshot: 'snap_b', added: 0, removed: 3, modified: 0, metric_names: ['metric_b'] },
    ]);
    expect(detail).toContain('**snap_a**');
    expect(detail).toContain('**snap_b**');
    expect(detail).toContain('2 added');
    expect(detail).toContain('3 removed');
  });

  test('includes link to CI run when ciRunUrl provided', () => {
    const detail = formatMetricsDetail([{
      snapshot: 'cassandra_v2',
      added: 1, removed: 0, modified: 0,
      metric_names: ['http_server_duration'],
    }], { ciRunUrl: 'https://github.com/org/repo/actions/runs/123' });
    expect(detail).toContain('[CI run](https://github.com/org/repo/actions/runs/123)');
    expect(detail).toContain('Compare metrics and generate summary');
  });

  test('omits CI run link when ciRunUrl not provided', () => {
    const detail = formatMetricsDetail([{
      snapshot: 'cassandra_v2',
      added: 1, removed: 0, modified: 0,
      metric_names: ['http_server_duration'],
    }]);
    expect(detail).not.toContain('[CI run]');
  });

  test('renders per-snapshot artifact download link when artifactUrlPrefix and artifact_id provided', () => {
    const detail = formatMetricsDetail([{
      snapshot: 'cassandra_v2',
      added: 1, removed: 0, modified: 0,
      metric_names: ['http_server_duration'],
      artifact_id: 6359406281,
    }], { artifactUrlPrefix: 'https://github.com/org/repo/actions/runs/123/artifacts' });
    expect(detail).toContain('[⬇️ download diff](https://github.com/org/repo/actions/runs/123/artifacts/6359406281)');
  });

  test('omits artifact download link when artifact_id is null', () => {
    const detail = formatMetricsDetail([{
      snapshot: 'cassandra_v2',
      added: 1, removed: 0, modified: 0,
      metric_names: ['http_server_duration'],
      artifact_id: null,
    }], { artifactUrlPrefix: 'https://github.com/org/repo/actions/runs/123/artifacts' });
    expect(detail).not.toContain('download diff');
  });

  test('omits artifact download link when artifactUrlPrefix not provided', () => {
    const detail = formatMetricsDetail([{
      snapshot: 'cassandra_v2',
      added: 1, removed: 0, modified: 0,
      metric_names: ['http_server_duration'],
      artifact_id: 6359406281,
    }]);
    expect(detail).not.toContain('download diff');
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

  test('does not include details block when no snapshots', () => {
    const body = buildCommentBody(metricsText, coverageText, footer);
    expect(body).not.toContain('<details>');
  });

  test('includes metrics detail when metricsSnapshots provided', () => {
    const snapshots = [{
      snapshot: 'cassandra_v2',
      added: 1, removed: 0, modified: 1,
      metric_names: ['http_server_duration'],
    }];
    const body = buildCommentBody('❌ 2 metric change(s) detected', coverageText, footer, { metricsSnapshots: snapshots });
    expect(body).toContain('<details>');
    expect(body).toContain('**cassandra_v2**');
    expect(body).toContain('- `http_server_duration`');
    expect(body).toContain('</details>');
    // Verify proper section ordering
    const metricsPos = body.indexOf('### Metrics Comparison');
    const detailsPos = body.indexOf('<details>');
    const coveragePos = body.indexOf('### Code Coverage');
    expect(metricsPos).toBeLessThan(detailsPos);
    expect(detailsPos).toBeLessThan(coveragePos);
  });

  test('includes CI run link in metrics detail when ciRunUrl provided', () => {
    const snapshots = [{
      snapshot: 'cassandra_v2',
      added: 1, removed: 0, modified: 0,
      metric_names: ['http_server_duration'],
      artifact_id: null,
    }];
    const ciRunUrl = 'https://github.com/org/repo/actions/runs/999';
    const body = buildCommentBody('❌ 1 metric change(s) detected', coverageText, footer, { metricsSnapshots: snapshots, ciRunUrl });
    expect(body).toContain(`[CI run](${ciRunUrl})`);
    expect(body).toContain('Compare metrics and generate summary');
  });

  test('includes per-snapshot artifact download link when artifactUrlPrefix and artifact_id provided', () => {
    const snapshots = [{
      snapshot: 'cassandra_v2',
      added: 1, removed: 0, modified: 0,
      metric_names: ['http_server_duration'],
      artifact_id: 6359406281,
    }];
    const artifactUrlPrefix = 'https://github.com/org/repo/actions/runs/999/artifacts';
    const body = buildCommentBody('❌ 1 metric change(s) detected', coverageText, footer, { metricsSnapshots: snapshots, artifactUrlPrefix });
    expect(body).toContain(`[⬇️ download diff](${artifactUrlPrefix}/6359406281)`);
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

  test('skips creating a new comment when createNew is false and no existing comment', async () => {
    const mockGithub = {
      paginate: jest.fn().mockResolvedValue([{ id: 1, body: 'unrelated' }]),
      rest: { issues: {
        listComments: jest.fn(),
        createComment: jest.fn(),
      }},
    };
    const mockCore = { info: jest.fn() };

    await postOrUpdateComment(mockGithub, owner, repo, prNumber, body, mockCore, { createNew: false });

    expect(mockGithub.rest.issues.createComment).not.toHaveBeenCalled();
    expect(mockCore.info).toHaveBeenCalledWith(expect.stringContaining('No existing comment'));
  });

  test('still updates existing comment when createNew is false', async () => {
    const existingComment = { id: 100, body: `${COMMENT_TAG}\nold failure` };
    const mockGithub = {
      paginate: jest.fn().mockResolvedValue([existingComment]),
      rest: { issues: {
        listComments: jest.fn(),
        updateComment: jest.fn().mockResolvedValue({ data: { html_url: 'https://example.com/comment/100' } }),
      }},
    };
    const mockCore = { info: jest.fn() };

    await postOrUpdateComment(mockGithub, owner, repo, prNumber, body, mockCore, { createNew: false });

    expect(mockGithub.rest.issues.updateComment).toHaveBeenCalledWith(
      expect.objectContaining({ owner, repo, comment_id: 100, body })
    );
    expect(mockCore.info).toHaveBeenCalledWith(expect.stringContaining('Updating existing comment id=100'));
  });
});
