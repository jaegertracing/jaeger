#!/usr/bin/env python3
# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
#
# Filters a Go coverage profile in-place by applying the same exclusions defined
# in .codecov.yml so coverage metrics stay in sync between this gate and Codecov.
#
# Usage:
#   python3 scripts/e2e/filter_coverage.py <coverage.out> [path/to/.codecov.yml]

import fnmatch
import sys


def load_exclusions(codecov_path: str) -> list[str]:
    """Return raw glob patterns from the ignore: section of .codecov.yml."""
    patterns = []
    in_ignore = False
    with open(codecov_path) as f:
        for line in f:
            stripped = line.strip()
            if stripped == 'ignore:':
                in_ignore = True
            elif in_ignore:
                if stripped.startswith('#'):
                    continue
                if stripped.startswith('- '):
                    patterns.append(stripped[2:].strip('"').strip("'"))
                elif stripped and not line[0].isspace():
                    in_ignore = False
    return patterns


def should_exclude(path: str, patterns: list[str]) -> bool:
    """Return True if path matches any exclusion pattern.

    Patterns with wildcards are matched via fnmatch (where * matches any
    sequence of characters, including /). Patterns without wildcards are
    treated as plain path substrings (directory prefixes).
    """
    for pattern in patterns:
        if '*' in pattern or '?' in pattern:
            if fnmatch.fnmatch(path, pattern):
                return True
        else:
            if pattern in path:
                return True
    return False


def main() -> None:
    if len(sys.argv) < 2:
        print(f'usage: {sys.argv[0]} <coverage.out> [.codecov.yml]', file=sys.stderr)
        sys.exit(1)

    coverage_path = sys.argv[1]
    codecov_path = sys.argv[2] if len(sys.argv) > 2 else '.codecov.yml'

    try:
        exclusions = load_exclusions(codecov_path)
    except FileNotFoundError:
        print(f'error: {codecov_path} not found', file=sys.stderr)
        sys.exit(1)

    kept = skipped = 0
    kept_lines = []
    with open(coverage_path) as f:
        for line in f:
            if line.startswith('mode:'):
                kept_lines.append(line)
                continue
            # Coverage lines: "github.com/.../file.go:line.col,line.col stmts count"
            # Extract the file path (everything before the first colon).
            path = line.split(':')[0]
            if should_exclude(path, exclusions):
                skipped += 1
            else:
                kept_lines.append(line)
                kept += 1

    with open(coverage_path, 'w') as f:
        f.writelines(kept_lines)

    print(f'filter_coverage: kept {kept}, excluded {skipped} lines', file=sys.stderr)


if __name__ == '__main__':
    main()
