#!/usr/bin/env python3

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""
This script inserts a new release section into CHANGELOG.md with the provided
version, date, and changelog content. If no content is provided, it will insert the
placeholder text.
"""

import argparse
import sys


def get_template_content():
    """
    Returns:
        str: The template content (without the version header)
    """
    with open('CHANGELOG.md', 'r') as f:
        lines = f.readlines()
    
    in_template = False
    template_content = []
    
    for line in lines:
        if '<summary>next release template</summary>' in line:
            in_template = True
            continue
        if '</details>' in line and in_template:
            break
        if in_template and not line.startswith('vX.Y.Z') and not line.startswith('---'): # Skip the version line and separator line
            template_content.append(line)
    
    return ''.join(template_content)


def update_changelog(version: str, release_date: str, changelog_content: str = "") -> None:
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
    new_section.append("\n")
    
    if changelog_content:
        new_section.append(changelog_content)
        if not changelog_content.endswith('\n'):
            new_section.append("\n")
    else:
        # Use the template content from CHANGELOG.md
        template = get_template_content()
        new_section.append(template)
    
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
        default=""
    )
    
    args = parser.parse_args()
    
    # Use provided date or default to today
    from datetime import date
    release_date = args.date if args.date else date.today().strftime("%Y-%m-%d")
    
    update_changelog(args.version, release_date, args.content)


if __name__ == "__main__":
    main()
