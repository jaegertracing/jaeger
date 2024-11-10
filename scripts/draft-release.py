#!/usr/bin/env python3

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import argparse
import re
import subprocess

generic_release_header_pattern = re.compile(
    r".*(1\.\d+\.\d+)", flags=0
)

jaeger_release_header_pattern = re.compile(
    r".*(1\.\d+\.\d+) */ *(2\.\d+\.\d+) \(\d{4}-\d{2}-\d{2}\)", flags=0
)

underline_pattern = re.compile(r"^[-]+$", flags=0)


def main(title, repo):
    changelog_text, version_v1, version_v2 = get_changelog(repo)
    print(changelog_text)
    header = f"{title} v{version_v1}"
    if repo == "jaeger":
        header += f" / v{version_v2}"
    output_string = subprocess.check_output(
        [
            "gh",
            "release",
            "create",
            f"v{version_v1}",
            "--draft",
            "--title",
            header,
            "--repo",
            f"jaegertracing/{repo}",
            "-F",
            "-",
        ],
        input=changelog_text,
        text=True,
    )
    print(f"Draft created at: {output_string}")
    print("Please review, then edit it and click 'Publish release'.")


def get_changelog(repo):
    changelog_text = ""
    in_changelog_text = False
    version_v1 = ""
    version_v2 = ""
    with open("CHANGELOG.md") as f:
        for line in f:
            release_header_match = generic_release_header_pattern.match(line)

            if release_header_match:
                # Found the first release.
                if in_changelog_text:
                    # Found the next release.
                    break
                else:
                    if repo == "jaeger":
                        jaeger_release_match = jaeger_release_header_pattern.match(line)
                        version_v1 = jaeger_release_match.group(1)
                        version_v2 = jaeger_release_match.group(2)
                    else:
                        version_v1 = release_header_match.group(1)
                    in_changelog_text = True
            else:
                underline_match = underline_pattern.match(line)
                if underline_match is not None:
                    continue
                elif in_changelog_text:
                    changelog_text += line

    return changelog_text, version_v1, version_v2


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

    args = parser.parse_args()

    main(args.title, args.repo)
