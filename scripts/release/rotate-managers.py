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


def increment_version(version_str: str) -> str:
    # version_str e.g. "2.14.0"
    parts = version_str.strip().split('.')
    if len(parts) != 3:
        return version_str # fallback

    major, minor, patch = map(int, parts)
    return f"{major}.{minor + 1}.0"


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
    header_plus_sep = lines[:2]
    data_lines = lines[2:]

    if not data_lines:
        print("Error: No data lines found in release managers table", file=sys.stderr)
        sys.exit(1)

    # Get the last row's version and date to calculate the next ones
    last_row = data_lines[-1]
    last_row_parts = [p.strip() for p in last_row.split('|') if p.strip()]
    if len(last_row_parts) < 3:
        print("Error: Could not parse last row of the table", file=sys.stderr)
        sys.exit(1)

    last_version = last_row_parts[0]
    last_date_str = last_row_parts[2]

    next_version = increment_version(last_version)
    next_date = get_next_first_wednesday(last_date_str)

    # Get the first row (the one to be rotated)
    first_row = data_lines[0]
    first_row_parts = [p.strip() for p in first_row.split('|') if p.strip()]
    manager = first_row_parts[1]

    # Create the new row for the bottom
    new_bottom_row = f"| {next_version:<7} | {manager:<15} | {next_date:<17} |"

    # Move first line to the end (rotation)
    rotated = data_lines[1:] + [new_bottom_row]
    new_table = '\n'.join(header_plus_sep + rotated)
    content = content[:match.start()] + new_table + content[match.end():]

    with open('RELEASE.md', 'w') as f:
        f.write(content)
    print(f"Rotated release managers table. Added {next_version} for {manager} on {next_date}")

def main():
    rotate_release_managers()

if __name__ == "__main__":
    main()
