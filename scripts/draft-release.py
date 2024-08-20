#!/usr/bin/env python3

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import argparse
import re
import subprocess


release_header_pattern = re.compile(r".*(\d+\.\d+\.\d) \(\d{4}-\d{2}-\d{2}\)", flags=0)
underline_pattern = re.compile(r"^[-]+$", flags=0)


def main(title, repo):
    changelog_text, version = get_changelog()
    print(changelog_text)
    output_string = subprocess.check_output(
        ["gh", "release", "create", f"v{version}",
         "--draft",
         "--title", f"{title} v{version}",
         "--repo", f"jaegertracing/{repo}",
         "-F", "-"],
        input=changelog_text,
        text=True,
    )
    print(f"Draft created at: {output_string}")
    print("Please review, then edit it and click 'Publish release'.")


def get_changelog():
    changelog_text = ""
    in_changelog_text = False
    version = ""
    with open("CHANGELOG.md") as f:
        for line in f:
            release_header_match = release_header_pattern.match(line)

            if release_header_match is not None:
                # Found the first release.
                if not in_changelog_text:
                    in_changelog_text = True
                    version = release_header_match.group(1)
                else:
                    # Found the next release.
                    break
            else:
                underline_match = underline_pattern.match(line)
                if underline_match is not None:
                    continue
                elif in_changelog_text:
                    changelog_text += line

    return changelog_text, version


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description='List changes based on git log for release notes.')

    parser.add_argument('--title', type=str, default='Release',
                        help='The title of the release. (default: Release)')
    parser.add_argument('--repo', type=str, default='jaeger',
                        help='The repository name where the draft release will be created. (default: jaeger)')

    args = parser.parse_args()

    main(args.title, args.repo)
