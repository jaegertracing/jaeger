# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import unittest
from compare_metrics import generate_diff, parse_metrics

# Minimal Prometheus text-format snippets used across tests.
_METRIC_A = '''\
# HELP counter_a A counter metric
# TYPE counter_a counter
counter_a_total{job="a"} 1
'''

_METRIC_B = '''\
# HELP counter_b Another counter metric
# TYPE counter_b counter
counter_b_total{job="b"} 1
'''

_METRIC_EXCLUDED_5XX = '''\
# HELP http_requests HTTP request counter
# TYPE http_requests counter
http_requests_total{http_response_status_code="500"} 1
'''

_METRIC_A_AND_EXCLUDED = _METRIC_A + _METRIC_EXCLUDED_5XX


class TestGenerateDiff(unittest.TestCase):
    """Tests for generate_diff() focusing on the two noise-reduction behaviours:

    1. Exclusion-count-only diffs (Cassandra issue):
       When the two snapshots contain the same non-excluded metrics but differ
       only in how many metrics were excluded (e.g. different numbers of 5xx
       responses captured), the diff must be empty — no false-positive report.

    2. New-metrics-only diffs (Elasticsearch / stale-baseline issue):
       When the current snapshot has metrics that the baseline does not, those
       new metrics should NOT trigger a failure.  Only metrics that are present
       in the baseline but *absent* from the current snapshot (regressions)
       should cause a non-empty diff.
    """

    def test_identical_snapshots_returns_empty(self):
        """Identical snapshots produce no diff."""
        result = generate_diff(_METRIC_A, _METRIC_A)
        self.assertEqual(result, '')

    def test_empty_snapshots_returns_empty(self):
        """Two empty snapshots produce no diff."""
        result = generate_diff('', '')
        self.assertEqual(result, '')

    def test_regression_detected(self):
        """Metric present in baseline but absent from current snapshot → diff is non-empty."""
        # current=A only, baseline=A+B → B is missing from current (regression)
        result = generate_diff(_METRIC_A, _METRIC_A + _METRIC_B)
        self.assertNotEqual(result, '', 'Expected a non-empty diff for a regression')
        # The diff must contain a '+' line for the missing metric (counter_b)
        self.assertIn('+counter_b', result)

    def test_new_metric_in_current_is_not_a_regression(self):
        """Metric present in current snapshot but absent from baseline → no diff.

        This covers the 'stale baseline' scenario: other PRs merged to main added
        new metrics after the last baseline was captured.  A PR that has those new
        metrics should not be penalised.
        """
        # current=A+B, baseline=A only → B is new in current, baseline is stale
        result = generate_diff(_METRIC_A + _METRIC_B, _METRIC_A)
        self.assertEqual(
            result,
            '',
            'New metrics in current snapshot should not produce a diff',
        )

    def test_exclusion_count_difference_does_not_produce_diff(self):
        """Snapshots that differ only in excluded-metric counts produce no diff.

        This covers the 'Cassandra false-positive' scenario: both snapshots have
        identical non-excluded metrics, but differ in how many 5xx-error metrics
        were excluded.  The exclusion-count header lines must not make the diff
        non-empty.
        """
        # current has metric_a + one 5xx (excluded), baseline has metric_a + zero 5xx
        result = generate_diff(_METRIC_A_AND_EXCLUDED, _METRIC_A)
        self.assertEqual(
            result,
            '',
            'Exclusion-count differences alone must not produce a non-empty diff',
        )

    def test_mixed_regression_and_new_metric_returns_diff(self):
        """When there is both a regression AND a new metric, the diff is non-empty."""
        # current=B only, baseline=A only → A is missing (regression), B is new
        result = generate_diff(_METRIC_B, _METRIC_A)
        self.assertNotEqual(result, '')
        self.assertIn('+counter_a', result)
        # The new metric should still appear in the raw diff output for visibility
        self.assertIn('-counter_b', result)

    def test_regression_with_exclusions_includes_exclusion_summary(self):
        """When there is a regression and excluded metrics, the output includes counts."""
        # current=excluded only, baseline=A+excluded → A is missing (regression)
        result = generate_diff(_METRIC_EXCLUDED_5XX, _METRIC_A + _METRIC_EXCLUDED_5XX)
        self.assertNotEqual(result, '')
        self.assertIn('Metrics excluded from A:', result)
        self.assertIn('Metrics excluded from B:', result)

    def test_no_exclusions_means_no_exclusion_summary(self):
        """When there are no excluded metrics, the exclusion summary is omitted."""
        result = generate_diff(_METRIC_A, _METRIC_A + _METRIC_B)
        self.assertNotIn('Metrics excluded from', result)


class TestParseMetrics(unittest.TestCase):
    """Smoke tests for parse_metrics() to verify label exclusion."""

    def test_excluded_labels_are_dropped(self):
        content = '''\
# HELP my_counter A counter
# TYPE my_counter counter
my_counter_total{service_instance_id="abc",job="x"} 1
'''
        metrics, _ = parse_metrics(content)
        self.assertTrue(any('my_counter' in m for m in metrics))
        # service_instance_id must have been removed
        self.assertFalse(any('service_instance_id' in m for m in metrics))

    def test_5xx_metrics_are_excluded(self):
        metrics, count = parse_metrics(_METRIC_EXCLUDED_5XX)
        self.assertEqual(metrics, [], 'Expected 5xx metric to be excluded')
        self.assertEqual(count, 1, 'Expected exclusion count of 1')


if __name__ == '__main__':
    unittest.main()
