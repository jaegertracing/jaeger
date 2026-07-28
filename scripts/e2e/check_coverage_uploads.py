#!/usr/bin/env python3
# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
#
# Checks the coverage uploads of a CI run against the Codecov notification
# threshold, so that a run which Codecov will never report on fails here with a
# diagnostic instead of stalling the merge queue silently.
#
# Codecov withholds every notification -- including the required codecov/patch
# and codecov/project statuses -- until `after_n_builds` uploads carrying
# non-ignored coverage have arrived. An upload whose profile covers only
# `ignore`-ed packages counts for nothing, so the threshold can stop being
# reachable without any job failing and without the count of uploading jobs
# changing.
#
# Usage:
#   python3 scripts/e2e/check_coverage_uploads.py <artifacts-dir> [path/to/.codecov.yml]

import glob
import os
import re
import sys

from filter_coverage import load_exclusions, read_module_path, should_exclude

# Emitted when a run cannot reach the threshold. GitHub renders ::error:: as an
# annotation on the job, which is the "loudly" half of the contract.
_ANNOTATION_ERROR = '::error::'
_ANNOTATION_WARNING = '::warning::'


def read_after_n_builds(codecov_path: str) -> int:
    """Return the after_n_builds value from .codecov.yml.

    Hand-parsed rather than via a YAML library to match filter_coverage.py and
    avoid a dependency in CI. Comment lines are skipped so prose mentioning the
    key does not shadow the setting.
    """
    with open(codecov_path) as f:
        for line in f:
            if line.strip().startswith('#'):
                continue
            match = re.match(r'\s*after_n_builds:\s*(\d+)', line)
            if match:
                return int(match.group(1))
    raise ValueError(f'no after_n_builds setting found in {codecov_path}')


def profile_contributes(profile_path: str, patterns: list[str], module_prefix: str) -> bool:
    """Return True if the profile has at least one line Codecov would count.

    Mirrors filter_coverage.py's exclusion logic, but per-profile and without
    rewriting the file: a profile left with nothing but its `mode:` header after
    exclusions is invisible to Codecov and does not advance after_n_builds.
    """
    with open(profile_path) as f:
        for line in f:
            if line.startswith('mode:') or not line.strip():
                continue
            import_path = line.split(':')[0]
            if import_path.startswith(module_prefix):
                path = import_path[len(module_prefix):]
            else:
                path = import_path
            if not should_exclude(path, patterns):
                return True
    return False


def find_uploads(artifacts_dir: str) -> dict[str, list[str]]:
    """Map each coverage-<flag> artifact to the profiles it contains.

    Mirrors the `*/coverage-*/*.out` glob the workflow merges, so this check and
    the coverage gate see the same set of uploads.
    """
    uploads: dict[str, list[str]] = {}
    pattern = os.path.join(artifacts_dir, '*', 'coverage-*', '*.out')
    for profile in sorted(glob.glob(pattern)) + sorted(
        glob.glob(os.path.join(artifacts_dir, 'coverage-*', '*.out'))
    ):
        flag = os.path.basename(os.path.dirname(profile))
        uploads.setdefault(flag, []).append(profile)
    return uploads


def evaluate(
    uploads: dict[str, list[str]], patterns: list[str], module_prefix: str, threshold: int
) -> tuple[list[str], list[str], bool]:
    """Partition uploads into contributing and empty, and decide whether to fail.

    Returns (contributing_flags, empty_flags, should_fail).

    The run fails only when Codecov was reachable and is not: at least
    `threshold` jobs uploaded, yet fewer than `threshold` of them carry
    countable coverage. A run with fewer uploads than the threshold ran a
    reduced matrix -- Codecov will not report on it either, but that is a
    property of the run shape rather than a regression this check introduced,
    so it is reported as a notice.
    """
    contributing = []
    empty = []
    for flag, profiles in sorted(uploads.items()):
        if any(profile_contributes(p, patterns, module_prefix) for p in profiles):
            contributing.append(flag)
        else:
            empty.append(flag)
    should_fail = len(uploads) >= threshold and len(contributing) < threshold
    return contributing, empty, should_fail


def main() -> None:
    if len(sys.argv) < 2:
        print(f'usage: {sys.argv[0]} <artifacts-dir> [.codecov.yml]', file=sys.stderr)
        sys.exit(1)

    artifacts_dir = sys.argv[1]
    codecov_path = sys.argv[2] if len(sys.argv) > 2 else '.codecov.yml'

    threshold = read_after_n_builds(codecov_path)
    patterns = load_exclusions(codecov_path)
    module_prefix = read_module_path(codecov_path) + '/'

    uploads = find_uploads(artifacts_dir)
    if not uploads:
        print('no coverage uploads found; nothing to check')
        return

    contributing, empty, should_fail = evaluate(uploads, patterns, module_prefix, threshold)

    print(f'coverage uploads: {len(uploads)}')
    print(f'carrying countable coverage: {len(contributing)}')
    print(f'after_n_builds: {threshold}')

    if empty:
        # A standing defect rather than a fresh regression: these jobs upload a
        # profile that covers only `ignore`-ed packages, so they pad the upload
        # count without advancing the threshold.
        print(
            f'{_ANNOTATION_WARNING}{len(empty)} coverage upload(s) carry no countable '
            f'coverage and do not advance after_n_builds: {", ".join(empty)}'
        )

    if should_fail:
        print(
            f'{_ANNOTATION_ERROR}Codecov cannot report on this run: after_n_builds is '
            f'{threshold} but only {len(contributing)} of {len(uploads)} uploads carry '
            f'countable coverage. Codecov withholds codecov/patch and codecov/project '
            f'until the threshold is met, so every PR would block with no failing check. '
            f'Fix whichever is wrong: restore coverage to the uploads listed above, or '
            f'set after_n_builds to {len(contributing)} in .codecov.yml.'
        )
        sys.exit(1)

    if len(uploads) < threshold:
        print(
            f'::notice::reduced matrix: {len(uploads)} upload(s) is below after_n_builds '
            f'({threshold}), so Codecov will not report on this run'
        )


if __name__ == '__main__':
    main()
