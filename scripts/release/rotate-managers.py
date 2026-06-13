#!/usr/bin/env python3

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""
This script finds the release managers table in RELEASE.md and move the first data row to the end, rotating the release manager schedule.
"""

import re
import sys
from datetime import datetime, timedelta


def get_next_first_wednesday(last_date_str: str) -> str:
    # last_date_str e.g. "7 July 2026"
    last_date = datetime.strptime(last_date_str.strip(), "%d %B %Y")
    # Move to the next month
    if last_date.month == 12:
        next_month = last_date.replace(year=last_date.year + 1, month=1, day=1)
    else:
        next_month = last_date.replace(month=last_date.month + 1, day=1)

    # Find the first Wednesday
    # weekday() is 0 for Monday, 2 for Wednesday
    days_to_wednesday = (2 - next_month.weekday() + 7) % 7
    first_wednesday = next_month + timedelta(days=days_to_wednesday)

    return first_wednesday.strftime("%-d %B %Y")


def rotate_release_managers() -> None:
    with open('RELEASE.md', 'r') as f:
        content = f.read()

    # Find the release managers table
    # Matches the header, separator, and all data rows.
    # The last row might not have a trailing newline.
    table_pattern = r'(\| Version \| Release Manager \| Tentative release date *\|\n\|-+\|.*\|\n(?:\|.*\|.*\|(?:\n|$))*)'
    match = re.search(table_pattern, content)

    if not match:
        print("Error: Could not find release managers table", file=sys.stderr)
        sys.exit(1)

    table = match.group(0)
    # Ensure we preserve the exact line endings by not stripping the match if we don't need to.
    # But for rotation, we need the lines.
    lines = table.splitlines()

    # skip header and separator
    header_plus_sep = lines[:2]
    data_lines = lines[2:]

    if not data_lines:
        print("Error: No data lines found in release managers table", file=sys.stderr)
        sys.exit(1)

    # Find the maximum version currently in the table to determine the next one
    max_major, max_minor, max_patch = -1, -1, -1
    last_date_str = ""

    for line in data_lines:
        # Split and filter to get clean parts
        parts = [p.strip() for p in line.split('|') if p.strip()]
        if len(parts) >= 3:
            v_str = parts[0]
            d_str = parts[2]
            v_parts = v_str.split('.')
            if len(v_parts) == 3:
                try:
                    major, minor, patch = map(int, v_parts)
                    if (major, minor, patch) > (max_major, max_minor, max_patch):
                        max_major, max_minor, max_patch = major, minor, patch
                        last_date_str = d_str
                except ValueError:
                    continue

    if max_major == -1:
        print("Error: Could not find any valid versions in the table", file=sys.stderr)
        sys.exit(1)

    next_version = f"{max_major}.{max_minor + 1}.0"
    next_date = get_next_first_wednesday(last_date_str)

    # Get the first row (the one to be rotated)
    first_row = data_lines[0]
    first_row_parts = [p.strip() for p in first_row.split('|') if p.strip()]
    if len(first_row_parts) < 2:
        print(f"Error: First data row is malformed (expected at least 2 columns): {first_row}", file=sys.stderr)
        sys.exit(1)
    manager = first_row_parts[1]

    # Create the new row for the bottom
    # Version (7) + Manager (15) + Date (25)
    # Total width with spaces: (1+7+1) + (1+15+1) + (1+25+1) = 9 + 17 + 27 = 53 dashes/chars.
    new_bottom_row = f"| {next_version:<7} | {manager:<15} | {next_date:<25} |"

    # Move first line to the end (rotation)
    rotated = data_lines[1:] + [new_bottom_row]
    # Reconstruct the table with a trailing newline to avoid corruption when joining back
    new_table = '\n'.join(header_plus_sep + rotated) + '\n'
    content = content[:match.start()] + new_table + content[match.end():]

    with open('RELEASE.md', 'w') as f:
        f.write(content)
    print(f"Rotated release managers table. Added {next_version} for {manager} on {next_date}")

def main():
    rotate_release_managers()

if __name__ == "__main__":
    main()
