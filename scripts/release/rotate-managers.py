#!/usr/bin/env python3

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""
This script finds the release managers table in RELEASE.md and move the first data row to the end, rotating the release manager schedule.
"""

import re
import sys


def rotate_release_managers() -> None:
    with open('RELEASE.md', 'r') as f:
        content = f.read()
    
    # Find the release managers table
    table_pattern = r'(\| Version \| Release Manager \| Tentative release date \|\n\|-+\|.*\|\n(?:\|.*\|.*\|\n)+)'
    match = re.search(table_pattern, content, re.MULTILINE)
    
    if not match:
        print("Error: Could not find release managers table", file=sys.stderr)
        sys.exit(1)
    
    table = match.group(0)
    lines = table.strip().split('\n')
    
    # skip header and separator
    data_lines = lines[2:]
    
    if not data_lines:
        print("Error: No data lines found in release managers table", file=sys.stderr)
        sys.exit(1)
    
    # Move first line to the end (rotation)
    rotated = data_lines[1:] + [data_lines[0]]
    new_table = '\n'.join(lines[:2] + rotated)
    content = content[:match.start()] + new_table + content[match.end():]
    
    with open('RELEASE.md', 'w') as f:
        f.write(content)
    print("Rotated release managers table")

def main():
    rotate_release_managers()

if __name__ == "__main__":
    main()
