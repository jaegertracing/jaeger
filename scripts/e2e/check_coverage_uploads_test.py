# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import os
import tempfile
import unittest
from check_coverage_uploads import (
    evaluate,
    find_uploads,
    profile_contributes,
    read_after_n_builds,
)

_MODULE = 'github.com/jaegertracing/jaeger/'

# Mirrors the shape of the real ignore list: a bare prefix and a glob.
_PATTERNS = ['cmd/jaeger/internal/integration', '**/mocks/*']

_COUNTABLE = f"""\
mode: atomic
{_MODULE}internal/storage/v2/elasticsearch/writer.go:10.2,12.3 2 1
"""

# What the e2e cells upload: coverage confined to an `ignore`-ed package, which
# Codecov discards, so the upload does not advance after_n_builds.
_IGNORED_ONLY = f"""\
mode: atomic
{_MODULE}cmd/jaeger/internal/integration/e2e_test.go:20.2,22.3 2 1
"""

_HEADER_ONLY = 'mode: atomic\n'


def _write_upload(root: str, flag: str, content: str) -> None:
    directory = os.path.join(root, f'coverage-{flag}')
    os.makedirs(directory, exist_ok=True)
    with open(os.path.join(directory, 'cover.out'), 'w') as f:
        f.write(content)


class TestReadAfterNBuilds(unittest.TestCase):
    def test_reads_value(self):
        with tempfile.NamedTemporaryFile('w', suffix='.yml', delete=False) as f:
            f.write('codecov:\n  notify:\n    after_n_builds: 12\n')
            path = f.name
        self.addCleanup(os.unlink, path)
        self.assertEqual(read_after_n_builds(path), 12)

    def test_ignores_commented_mentions(self):
        """Prose above the setting must not shadow it -- the real file has both."""
        with tempfile.NamedTemporaryFile('w', suffix='.yml', delete=False) as f:
            f.write(
                'codecov:\n'
                '  notify:\n'
                '    # after_n_builds: 99 is what a stale comment might claim\n'
                '    after_n_builds: 12\n'
            )
            path = f.name
        self.addCleanup(os.unlink, path)
        self.assertEqual(read_after_n_builds(path), 12)

    def test_missing_setting_raises(self):
        with tempfile.NamedTemporaryFile('w', suffix='.yml', delete=False) as f:
            f.write('codecov:\n  notify:\n    require_ci_to_pass: yes\n')
            path = f.name
        self.addCleanup(os.unlink, path)
        with self.assertRaises(ValueError):
            read_after_n_builds(path)


class TestProfileContributes(unittest.TestCase):
    def _check(self, content: str) -> bool:
        with tempfile.NamedTemporaryFile('w', suffix='.out', delete=False) as f:
            f.write(content)
            path = f.name
        self.addCleanup(os.unlink, path)
        return profile_contributes(path, _PATTERNS, _MODULE)

    def test_non_ignored_line_counts(self):
        self.assertTrue(self._check(_COUNTABLE))

    def test_ignored_only_does_not_count(self):
        self.assertFalse(self._check(_IGNORED_ONLY))

    def test_header_only_does_not_count(self):
        self.assertFalse(self._check(_HEADER_ONLY))

    def test_glob_pattern_excluded(self):
        self.assertFalse(self._check(f'mode: atomic\n{_MODULE}internal/mocks/foo.go:1.1,2.2 1 1\n'))


class TestFindUploads(unittest.TestCase):
    def test_groups_profiles_by_flag(self):
        with tempfile.TemporaryDirectory() as root:
            _write_upload(root, 'unittests', _COUNTABLE)
            _write_upload(root, 'elasticsearch-9.x-e2e', _IGNORED_ONLY)
            uploads = find_uploads(root)
        self.assertEqual(
            sorted(uploads), ['coverage-elasticsearch-9.x-e2e', 'coverage-unittests']
        )

    def test_ignores_non_coverage_artifacts(self):
        with tempfile.TemporaryDirectory() as root:
            _write_upload(root, 'unittests', _COUNTABLE)
            os.makedirs(os.path.join(root, 'metrics_snapshot_es'), exist_ok=True)
            with open(os.path.join(root, 'metrics_snapshot_es', 'snap.out'), 'w') as f:
                f.write('not coverage\n')
            uploads = find_uploads(root)
        self.assertEqual(list(uploads), ['coverage-unittests'])


class TestEvaluate(unittest.TestCase):
    def _evaluate(self, spec: dict[str, str], threshold: int):
        with tempfile.TemporaryDirectory() as root:
            for flag, content in spec.items():
                _write_upload(root, flag, content)
            uploads = find_uploads(root)
            return evaluate(uploads, _PATTERNS, _MODULE, threshold)

    def test_threshold_met_passes(self):
        contributing, empty, should_fail = self._evaluate(
            {'a': _COUNTABLE, 'b': _COUNTABLE}, threshold=2
        )
        self.assertEqual(len(contributing), 2)
        self.assertEqual(empty, [])
        self.assertFalse(should_fail)

    def test_current_repository_shape_passes(self):
        """Today's state: padding uploads exist, but the threshold is still met."""
        spec = {f'real{i}': _COUNTABLE for i in range(12)}
        spec.update({f'e2e{i}': _IGNORED_ONLY for i in range(15)})
        contributing, empty, should_fail = self._evaluate(spec, threshold=12)
        self.assertEqual(len(contributing), 12)
        self.assertEqual(len(empty), 15)
        self.assertFalse(should_fail)

    def test_regression_below_threshold_fails(self):
        """The outage condition: enough uploads, too few carrying coverage."""
        spec = {f'real{i}': _COUNTABLE for i in range(9)}
        spec.update({f'e2e{i}': _IGNORED_ONLY for i in range(18)})
        contributing, empty, should_fail = self._evaluate(spec, threshold=12)
        self.assertEqual(len(contributing), 9)
        self.assertTrue(should_fail)

    def test_reduced_matrix_does_not_fail(self):
        """Fewer uploads than the threshold is a run shape, not a regression."""
        contributing, empty, should_fail = self._evaluate(
            {'a': _COUNTABLE, 'b': _COUNTABLE}, threshold=12
        )
        self.assertEqual(len(contributing), 2)
        self.assertFalse(should_fail)

    def test_all_empty_with_enough_uploads_fails(self):
        spec = {f'e2e{i}': _IGNORED_ONLY for i in range(12)}
        contributing, empty, should_fail = self._evaluate(spec, threshold=12)
        self.assertEqual(contributing, [])
        self.assertEqual(len(empty), 12)
        self.assertTrue(should_fail)


if __name__ == '__main__':
    unittest.main()
