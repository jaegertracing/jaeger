#!/usr/bin/env python3

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""
This script inserts a new release section into CHANGELOG.md with the provided
version, date, and changelog content. If no content is provided, it will insert the
placeholder text.
"""

import argparse
import os
import sys


def extract_version_content(changelog_path: str, version: str) -> str:
    """
    Extracts the content of a specific version section from a changelog file.
    """
    if not os.path.exists(changelog_path):
        return ""

    with open(changelog_path, 'r') as f:
        lines = f.readlines()

    start_line = -1
    v_header = f"## v{version}"
    for i, line in enumerate(lines):
        if line.startswith(v_header):
            start_line = i + 1
            break

    if start_line == -1:
        return ""

    content = []
    for i in range(start_line, len(lines)):
        if lines[i].startswith('## v'):
            break
        content.append(lines[i])

    return ''.join(content).strip()


def update_changelog(version: str, release_date: str, changelog_content: str, ui_changelog: str = None) -> None:
    with open('CHANGELOG.md', 'r') as f:
        lines = f.readlines()

    # Find the template section end
    template_end = -1
    for i, line in enumerate(lines):
        if '</details>' in line:
            template_end = i + 1
            break
    
    if template_end == -1:
        print("Error: Could not find template end marker", file=sys.stderr)
        sys.exit(1)
    
    # Create the new changelog section
    new_section = []
    new_section.append(f"\nv{version} ({release_date})\n")
    new_section.append("-" * 31 + "\n")
    if not changelog_content.startswith('\n'):
        new_section.append("\n")
    new_section.append(changelog_content)
    if not changelog_content.endswith('\n'):
        new_section.append("\n")
    
    if ui_changelog:
        ui_content = extract_version_content(ui_changelog, version)
        if ui_content:
            new_section.append("\n### 📊 UI Changes\n\n")
            new_section.append(ui_content)
            new_section.append("\n")

    with open('CHANGELOG.md', 'w') as f: # Write the updated CHANGELOG.md
        f.writelines(lines[:template_end])
        f.writelines(new_section)
        f.writelines(lines[template_end:])
    
    print(f"Updated CHANGELOG.md with v{version}")


def main():
    parser = argparse.ArgumentParser(
        description="Update CHANGELOG.md with a new version section."
    )
    parser.add_argument(
        "version",
        type=str,
        help="Version number (e.g., 2.14.0)"
    )
    parser.add_argument(
        "--date",
        type=str,
        help="Release date in YYYY-MM-DD format (default: today)",
        default=None
    )
    parser.add_argument(
        "--content",
        type=str,
        help="Changelog content (default: placeholder text)",
        default=None
    )
    parser.add_argument(
        "--ui-changelog",
        type=str,
        help="Path to the UI changelog file to extract notes from",
        default=None
    )
    
    args = parser.parse_args()
    
    # Use provided date or default to today
    from datetime import date
    release_date = args.date if args.date else date.today().strftime("%Y-%m-%d")
    
    update_changelog(args.version, release_date, args.content, args.ui_changelog)


if __name__ == "__main__":
    main()
