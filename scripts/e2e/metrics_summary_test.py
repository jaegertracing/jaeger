# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import os
import tempfile
import unittest
from metrics_summary import (
    extract_metric_name,
    get_raw_diff_sample,
    parse_diff_file,
    generate_diff_summary,
    generate_structured_json,
)


# A minimal unified diff that exercises added/removed/modified cases and exclusion counts.
_DIFF_WITH_ALL_CATEGORIES = """\
--- 
+++ 
@@ -1,3 +1,3 @@
-baseline_only{job="a"}
-http_server_duration{le="+Inf"}
+current_only{job="b"}
+http_server_duration{http_route="/status",le="+Inf"}
# Metrics excluded from current: 2
# Metrics excluded from baseline: 3
"""

# A diff with only added metrics (present in current, absent from baseline).
_DIFF_ADDED_ONLY = """\
--- 
+++ 
@@ -1 +2 @@
+new_metric{job="a"}
"""

# A diff with only removed metrics (present in baseline, absent from current).
_DIFF_REMOVED_ONLY = """\
--- 
+++ 
@@ -2 +1 @@
-old_metric{job="b"}
"""


def _write_tmp(content):
    """Write content to a temp file and return its path."""
    f = tempfile.NamedTemporaryFile(mode='w', suffix='.txt', delete=False)
    f.write(content)
    f.close()
    return f.name


class TestExtractMetricName(unittest.TestCase):
    """Tests for extract_metric_name()."""

    def test_extracts_name_before_braces(self):
        self.assertEqual(extract_metric_name('http_server_duration{le="+Inf"}'), 'http_server_duration')

    def test_returns_bare_name_when_no_braces(self):
        self.assertEqual(extract_metric_name('my_metric'), 'my_metric')

    def test_strips_whitespace(self):
        self.assertEqual(extract_metric_name('  my_metric  '), 'my_metric')


class TestGetRawDiffSample(unittest.TestCase):
    """Tests for get_raw_diff_sample()."""

    def test_empty_input_returns_empty(self):
        self.assertEqual(get_raw_diff_sample([]), [])

    def test_pure_removals_are_truncated(self):
        lines = [f'-metric_{i}' for i in range(10)]
        result = get_raw_diff_sample(lines, max_lines=4)
        self.assertEqual(result[:4], lines[:4])
        self.assertEqual(result[-1], '...')

    def test_pure_additions_are_truncated(self):
        lines = [f'+metric_{i}' for i in range(10)]
        result = get_raw_diff_sample(lines, max_lines=4)
        self.assertEqual(result[:4], lines[:4])
        self.assertEqual(result[-1], '...')

    def test_interleaves_removed_and_added_lines(self):
        """Modified metrics with both - and + lines are interleaved."""
        removed = [f'-metric{{a="{i}"}}' for i in range(4)]
        added   = [f'+metric{{a="{i}",b="x"}}' for i in range(4)]
        result = get_raw_diff_sample(removed + added, max_lines=4)
        # Pairs: (-, +, -, +, ...)
        self.assertTrue(result[0].startswith('-'))
        self.assertTrue(result[1].startswith('+'))
        self.assertTrue(result[2].startswith('-'))
        self.assertTrue(result[3].startswith('+'))

    def test_interleaved_truncation_adds_ellipsis(self):
        removed = [f'-metric{{a="{i}"}}' for i in range(10)]
        added   = [f'+metric{{a="{i}",b="x"}}' for i in range(10)]
        result = get_raw_diff_sample(removed + added, max_lines=4)
        self.assertIn('...', result)
        # Should only show max_lines//2 = 2 pairs
        non_ellipsis = [l for l in result if l != '...']
        self.assertEqual(len(non_ellipsis), 4)  # 2 pairs × 2 lines each

    def test_no_ellipsis_when_within_limit(self):
        lines = ['-m1', '+m1_v2']
        result = get_raw_diff_sample(lines, max_lines=8)
        self.assertNotIn('...', result)


class TestParseDiffFile(unittest.TestCase):
    """Tests for parse_diff_file()."""

    def test_parses_added_removed_and_modified(self):
        path = _write_tmp(_DIFF_WITH_ALL_CATEGORIES)
        try:
            changes, raw_sections, excl_count = parse_diff_file(path)
        finally:
            os.unlink(path)

        # baseline_only is in baseline but not current → removed
        self.assertIn('baseline_only', changes['removed'])
        # current_only is in current but not baseline → added
        self.assertIn('current_only', changes['added'])
        # http_server_duration appears in both → modified
        self.assertIn('http_server_duration', changes['modified'])
        # Not in added or removed after reclassification
        self.assertNotIn('http_server_duration', changes['added'])
        self.assertNotIn('http_server_duration', changes['removed'])

    def test_accumulates_exclusion_counts(self):
        path = _write_tmp(_DIFF_WITH_ALL_CATEGORIES)
        try:
            _, _, excl_count = parse_diff_file(path)
        finally:
            os.unlink(path)
        self.assertEqual(excl_count, 5)  # 2 + 3

    def test_zero_exclusion_count_when_no_exclusion_lines(self):
        path = _write_tmp(_DIFF_ADDED_ONLY)
        try:
            _, _, excl_count = parse_diff_file(path)
        finally:
            os.unlink(path)
        self.assertEqual(excl_count, 0)

    def test_raw_diff_sections_populated(self):
        path = _write_tmp(_DIFF_WITH_ALL_CATEGORIES)
        try:
            _, raw_sections, _ = parse_diff_file(path)
        finally:
            os.unlink(path)
        self.assertIn('http_server_duration', raw_sections)
        # The section should have at least one - line and one + line
        lines = raw_sections['http_server_duration']
        self.assertTrue(any(l.startswith('-') for l in lines))
        self.assertTrue(any(l.startswith('+') for l in lines))

    def test_added_only_diff(self):
        path = _write_tmp(_DIFF_ADDED_ONLY)
        try:
            changes, _, _ = parse_diff_file(path)
        finally:
            os.unlink(path)
        self.assertIn('new_metric', changes['added'])
        self.assertEqual(len(changes['removed']), 0)
        self.assertEqual(len(changes['modified']), 0)

    def test_removed_only_diff(self):
        path = _write_tmp(_DIFF_REMOVED_ONLY)
        try:
            changes, _, _ = parse_diff_file(path)
        finally:
            os.unlink(path)
        self.assertIn('old_metric', changes['removed'])
        self.assertEqual(len(changes['added']), 0)
        self.assertEqual(len(changes['modified']), 0)


class TestGenerateDiffSummary(unittest.TestCase):
    """Tests for generate_diff_summary()."""

    def test_total_changes_header_present(self):
        path = _write_tmp(_DIFF_WITH_ALL_CATEGORIES)
        try:
            changes, raw_sections, excl_count = parse_diff_file(path)
        finally:
            os.unlink(path)
        summary = generate_diff_summary(changes, raw_sections, excl_count)
        self.assertIn('**Total Changes:**', summary)

    def test_added_section_rendered(self):
        path = _write_tmp(_DIFF_ADDED_ONLY)
        try:
            changes, raw_sections, excl_count = parse_diff_file(path)
        finally:
            os.unlink(path)
        summary = generate_diff_summary(changes, raw_sections, excl_count)
        self.assertIn('Added Metrics', summary)
        self.assertIn('new_metric', summary)

    def test_removed_section_rendered(self):
        path = _write_tmp(_DIFF_REMOVED_ONLY)
        try:
            changes, raw_sections, excl_count = parse_diff_file(path)
        finally:
            os.unlink(path)
        summary = generate_diff_summary(changes, raw_sections, excl_count)
        self.assertIn('Removed Metrics', summary)
        self.assertIn('old_metric', summary)

    def test_modified_section_rendered(self):
        path = _write_tmp(_DIFF_WITH_ALL_CATEGORIES)
        try:
            changes, raw_sections, excl_count = parse_diff_file(path)
        finally:
            os.unlink(path)
        summary = generate_diff_summary(changes, raw_sections, excl_count)
        self.assertIn('Modified Metrics', summary)
        self.assertIn('http_server_duration', summary)

    def test_diff_sample_block_present_for_changed_metric(self):
        path = _write_tmp(_DIFF_WITH_ALL_CATEGORIES)
        try:
            changes, raw_sections, excl_count = parse_diff_file(path)
        finally:
            os.unlink(path)
        summary = generate_diff_summary(changes, raw_sections, excl_count)
        self.assertIn('```diff', summary)

    def test_exclusion_count_shown(self):
        path = _write_tmp(_DIFF_WITH_ALL_CATEGORIES)
        try:
            changes, raw_sections, excl_count = parse_diff_file(path)
        finally:
            os.unlink(path)
        summary = generate_diff_summary(changes, raw_sections, excl_count)
        self.assertIn('Excluded: 5', summary)


class TestGenerateStructuredJson(unittest.TestCase):
    """Tests for generate_structured_json()."""

    def test_counts_are_correct(self):
        path = _write_tmp(_DIFF_WITH_ALL_CATEGORIES)
        try:
            changes, _, _ = parse_diff_file(path)
        finally:
            os.unlink(path)
        data = generate_structured_json(changes)
        self.assertEqual(data['added'], 1)    # current_only
        self.assertEqual(data['removed'], 1)  # baseline_only
        self.assertEqual(data['modified'], 1) # http_server_duration

    def test_metric_names_list_sorted(self):
        path = _write_tmp(_DIFF_WITH_ALL_CATEGORIES)
        try:
            changes, _, _ = parse_diff_file(path)
        finally:
            os.unlink(path)
        data = generate_structured_json(changes)
        names = data['metric_names']
        self.assertEqual(names, sorted(names))

    def test_metric_names_deduped(self):
        path = _write_tmp(_DIFF_WITH_ALL_CATEGORIES)
        try:
            changes, _, _ = parse_diff_file(path)
        finally:
            os.unlink(path)
        data = generate_structured_json(changes)
        self.assertEqual(len(data['metric_names']), len(set(data['metric_names'])))

    def test_added_only_produces_correct_output(self):
        path = _write_tmp(_DIFF_ADDED_ONLY)
        try:
            changes, _, _ = parse_diff_file(path)
        finally:
            os.unlink(path)
        data = generate_structured_json(changes)
        self.assertEqual(data['added'], 1)
        self.assertEqual(data['removed'], 0)
        self.assertEqual(data['modified'], 0)
        self.assertIn('new_metric', data['metric_names'])


if __name__ == '__main__':
    unittest.main()
