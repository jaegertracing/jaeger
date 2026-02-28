#!/usr/bin/env python3
# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
#
# Filters a Go coverage profile by applying the same exclusions defined in
# .codecov.yml so coverage metrics stay in sync between this gate and Codecov.
#
# Usage:
#   python3 scripts/e2e/filter_coverage.py <coverage.out> [path/to/.codecov.yml]
#
# Writes the filtered profile to stdout; redirect to a file as needed.

import re
import sys


def glob_to_regex(glob: str) -> re.Pattern:
    """Convert a Codecov glob pattern to a compiled regex for substring matching."""
    p = glob
    # Remove leading **/ or **
    p = re.sub(r'^\*\*/?', '', p)
    # Escape dots before other substitutions
    p = p.replace('.', r'\.')
    # Remove trailing /* or /**  (directory wildcard suffix)
    p = re.sub(r'/\*+$', '/', p)
    # Replace remaining ** with .*
    p = p.replace('**', '.*')
    # Replace remaining * with [^/]* (single path-segment wildcard)
    p = p.replace('*', r'[^/]*')
    return re.compile(p)


def load_exclusions(codecov_path: str) -> list[re.Pattern]:
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
                    raw = stripped[2:].strip('"').strip("'")
                    patterns.append(glob_to_regex(raw))
                elif stripped and not line[0].isspace():
                    in_ignore = False
    return patterns


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
    with open(coverage_path) as f:
        for line in f:
            # Always keep the mode header line
            if line.startswith('mode:'):
                sys.stdout.write(line)
                continue
            if any(p.search(line) for p in exclusions):
                skipped += 1
            else:
                sys.stdout.write(line)
                kept += 1

    print(f'filter_coverage: kept {kept}, excluded {skipped} lines', file=sys.stderr)


if __name__ == '__main__':
    main()
