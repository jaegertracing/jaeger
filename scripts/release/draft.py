#!/usr/bin/env python3

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import argparse
import re
import subprocess

version_pattern = re.compile(r"^[# ]*v?(\d+\.\d+\.\d+)")

underline_pattern = re.compile(r"^[-]+$", flags=0)


def main(title, repo, dry_run=False):
    changelog_text, version = get_changelog(repo)
    header = f"{title} v{version}"
    tag = f"v{version}"
    full_repo = f"jaegertracing/{repo}"

    if dry_run:
        print("Dry run: skipping release creation.")
        print(f"Repository: {full_repo}")
        print(f"Tag:        {tag}")
        print(f"Title:      {header}")
        print("Changelog:")
        print("-" * 20)
        print(changelog_text)
        print("-" * 20)
        return

    print(changelog_text)
    output_string = subprocess.check_output(
        [
            "gh",
            "release",
            "create",
            tag,
            "--draft",
            "--title",
            header,
            "--repo",
            full_repo,
            "-F",
            "-",
        ],
        input=changelog_text,
        text=True,
    )
    print(f"Draft created at: {output_string}")
    print("Please review, then edit it and click 'Publish release'.")


def get_changelog(repo):
# ... (rest of get_changelog remains the same)
    changelog_text = ""
    in_changelog_text = False
    version = ""
    with open("CHANGELOG.md") as f:
        for line in f:
            versions = version_pattern.findall(line)

            if versions:
                # Found the first release headers.
                if in_changelog_text:
                    # Found the next release.
                    break
                else:
                    # If both v1 and v2 are present, pick v2 (usually the last one).
                    # If only one version is present, pick it.
                    version = versions[-1]
                    in_changelog_text = True
            else:
                underline_match = underline_pattern.match(line)
                if underline_match is not None:
                    continue
                elif in_changelog_text:
                    changelog_text += line

    return changelog_text, version


if __name__ == "__main__":
    parser = argparse.ArgumentParser(
        description="List changes based on git log for release notes."
    )

    parser.add_argument(
        "--title",
        type=str,
        default="Release",
        help="The title of the release. (default: Release)",
    )
    parser.add_argument(
        "--repo",
        type=str,
        default="jaeger",
        help="The repository name where the draft release will be created. (default: jaeger)",
    )
    parser.add_argument(
        "-d",
        "--dry-run",
        action="store_true",
        help="Print the release details without creating it.",
    )

    args = parser.parse_args()

    main(args.title, args.repo, args.dry_run)
