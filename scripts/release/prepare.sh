#!/usr/bin/env bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# This script automates the Jaeger release preparation process.
# It creates a pull request with all the changes necessary for release:
#   1. Update CHANGELOG.md with the new version and auto-generated release notes.
#   2. Update the jaeger-ui submodule to the corresponding version
#   3. Rotate the release managers table in RELEASE.md
#   4. Create a PR with the 'changelog:skip' label
#   5. Include exact tag commands in the PR description for post-merge execution.
#
# Use:
#   bash scripts/release/prepare.sh <version>
#   OR
#   make prepare-release VERSION=<version>
#
# Example:
#   bash scripts/release/prepare.sh 2.14.0
#   make prepare-release VERSION=2.14.0
#
# After the PR is merged, follow the tag commands in the PR description.

set -euo pipefail

check_prerequisites() {
    for tool in gh git python3; do
        if ! command -v "$tool" &> /dev/null; then
            echo "Error: $tool is not installed or not in PATH"
            exit 1
        fi
    done
}

# Verify we're on main branch
verify_on_main_branch() {
    local current_branch

    current_branch=$(git rev-parse --abbrev-ref HEAD)
    if [ "$current_branch" != "main" ]; then
        echo "Warning: Not on main branch (current: ${current_branch})"
        read -p "Continue anyway? (y/n) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 1
        fi
    fi
}

# find by checking URL patterns
fetch_from_official_remote() {
    local official_remote

    if ! official_remote=$(bash scripts/utils/find-official-remote.sh); then
        exit 1
    fi

    echo "Fetching from official repo: $official_remote"
    git fetch "$official_remote"
}

# Create a new branch
create_release_branch() {
    local version=$1
    local branch_name
    branch_name="prepare-release-v${version}-$(date +%s)"

    git checkout -b "${branch_name}"
    echo "$branch_name"
}

# Update UI submodule
update_ui_submodule() {
    local version=$1
    local ui_version="v${version}"

    echo "Updating UI submodule..."
    make init-submodules
    # Verify if the directory exists and is not empty
    if [ ! -d "jaeger-ui" ] || [ ! "$(ls -A jaeger-ui)" ]; then
        echo "Error: jaeger-ui directory does not exist or is empty"
        exit 1
    fi

    pushd jaeger-ui > /dev/null
    git fetch origin
    if git rev-parse "${ui_version}" >/dev/null 2>&1; then
        git checkout "${ui_version}"
        echo "Checked out jaeger-ui ${ui_version}"
    else
        # UI version not found
        echo "Warning: UI version ${ui_version} not found"
        read -r -p "Enter UI version to use, e.g. ${ui_version} (or Enter to skip): " ui_input
        if [ -n "$ui_input" ]; then
            git checkout "$ui_input"
        else
            echo "Skipping UI version update"
            git checkout main && git pull
        fi
    fi

    popd > /dev/null
    git add jaeger-ui
}

# Generate changelog entries and update CHANGELOG.md
update_changelog() {
    local version=$1
    local release_date
    local changelog_content

    echo "Updating CHANGELOG.md..."
    release_date=$(date +%Y-%m-%d)
    changelog_content=$(make -s changelog)

    python3 scripts/release/update-changelog.py "$version" --date "$release_date" --content "$changelog_content" --ui-changelog jaeger-ui/CHANGELOG.md
    git add CHANGELOG.md
}

rotate_release_managers() {
    echo "Rotating release managers table..."
    python3 scripts/release/rotate-managers.py
    # Stage RELEASE.md if it was modified
    git diff --quiet RELEASE.md || git add RELEASE.md
}

commit_changes() {
    local version=$1

    git commit -s -m "Prepare release v${version}

- Updated CHANGELOG.md with release notes
- Updated jaeger-ui submodule
- Rotated release managers table"
}

push_branch() {
    local branch_name=$1
    git push origin "${branch_name}"
}

create_pull_request() {
    local version=$1

    local pr_body="This PR prepares the release for v${version}.

## Changes
- [x] Updated CHANGELOG.md with release notes
- [x] Updated jaeger-ui submodule to v${version}
- [x] Rotated release managers table in RELEASE.md

After this PR is merged, continue with the release process as outlined in the release issue."

    # Create the PR
    gh pr create \
        --title "Prepare release v${version}" \
        --body "$pr_body" \
        --label "changelog:skip" \
        --base main
}

main() {
    if [ "$#" -ne 1 ]; then
        echo "Usage: $0 <version>"
        echo "Example: $0 2.14.0"
        exit 1
    fi

    local version="${1#v}" # Remove 'v' prefix if present

    echo "Preparing release for v${version}"

    check_prerequisites
    verify_on_main_branch
    fetch_from_official_remote

    local branch_name
    branch_name=$(create_release_branch "$version")

    update_ui_submodule "$version"
    update_changelog "$version"
    rotate_release_managers
    commit_changes "$version"
    push_branch "$branch_name"
    create_pull_request "$version"

    echo "Done. Review and merge the PR, then follow the instructions in the PR description."
}

main "$@"
