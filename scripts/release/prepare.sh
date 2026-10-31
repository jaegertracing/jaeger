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
#   bash scripts/release/prepare.sh <version> [issue]
#   OR
#   make prepare-release VERSION=<version> [ISSUE=<issue>]
#
# The optional issue number links the release PR back to the release
# tracking issue via a "Part of #<issue>" line in the PR description.
#
# Example:
#   bash scripts/release/prepare.sh 2.14.0 1234
#   make prepare-release VERSION=2.14.0 ISSUE=1234
#
# After the PR is merged, follow the tag commands in the PR description.

set -euo pipefail

official_remote=""
rotated_line=""

check_prerequisites() {
    for tool in gh git python3; do
        if ! command -v "$tool" &> /dev/null; then
            echo "Error: $tool is not installed or not in PATH"
            exit 1
        fi
    done
}

# Uncommitted changes to tracked files would be carried onto the release branch
verify_clean_worktree() {
    if [ -n "$(git status --porcelain --untracked-files=no --ignore-submodules)" ]; then
        echo "Error: working tree has uncommitted changes; commit or stash them first"
        git status --short --untracked-files=no --ignore-submodules
        exit 1
    fi
}

# find by checking URL patterns; sets the global official_remote
fetch_from_official_remote() {
    if ! official_remote=$(bash scripts/utils/find-official-remote.sh); then
        exit 1
    fi

    echo "Fetching from official repo: $official_remote"
    git fetch "$official_remote"
}

# Refuse to release while a feature gate scheduled for removal in this
# version is still registered.
check_due_feature_gates() {
    local version=$1
    local matches rc=0

    matches=$(git grep -nF "WithRegisterToVersion(\"v${version}\")" -- '*.go' ':!*_test.go') || rc=$?
    if [ "$rc" -gt 1 ]; then
        echo "Error: git grep failed (exit ${rc}) while checking feature gates"
        exit 1
    fi
    if [ "$rc" -eq 0 ]; then
        echo "Error: these feature gates are scheduled for removal in v${version} but are still registered:"
        echo "$matches"
        echo "Land the gate transitions due in v${version} first (see https://github.com/jaegertracing/jaeger/issues/9057)."
        exit 1
    fi
}

# Create a new branch from the official main, not the local checkout
create_release_branch() {
    local version=$1
    local branch_name
    branch_name="prepare-release-v${version}-$(date +%s)"

    # set -e is not in effect inside the $(...) this runs in, so exit explicitly
    git checkout --no-track -b "${branch_name}" "${official_remote}/main" >&2 || exit 1
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
        echo "Error: jaeger-ui tag ${ui_version} not found. Release jaeger-ui ${ui_version} first."
        exit 1
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
    local version=$1

    echo "Rotating release managers table..."
    python3 scripts/release/rotate-managers.py "$version"
    # Stage RELEASE.md if it was modified
    if ! git diff --quiet RELEASE.md; then
        git add RELEASE.md
        rotated_line="Rotated release managers table in RELEASE.md"
    fi
}

commit_changes() {
    local version=$1
    local msg="Prepare release v${version}

- Updated CHANGELOG.md with release notes
- Updated jaeger-ui submodule to v${version}"

    if [ -n "$rotated_line" ]; then
        msg="${msg}
- ${rotated_line}"
    fi
    git commit -s -m "$msg"
}

push_branch() {
    local branch_name=$1
    git push origin "${branch_name}"
}

create_pull_request() {
    local version=$1
    local issue=$2

    local pr_body="This PR prepares the release for v${version}.

## Changes
- [x] Updated CHANGELOG.md with release notes
- [x] Updated jaeger-ui submodule to v${version}${rotated_line:+
- [x] ${rotated_line}}

After this PR is merged, continue with the release process as outlined in the release issue."

    if [ -n "$issue" ]; then
        pr_body="${pr_body}

Part of #${issue}"
    fi

    # Create the PR
    gh pr create \
        -R jaegertracing/jaeger \
        --title "Prepare release v${version}" \
        --body "$pr_body" \
        --label "changelog:skip" \
        --base main
}

main() {
    if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
        echo "Usage: $0 <version> [issue]"
        echo "Example: $0 2.14.0 1234"
        exit 1
    fi

    local version="${1#v}" # Remove 'v' prefix if present
    local issue="${2:-}"
    issue="${issue#\#}"    # Remove '#' prefix if present
    if [ -n "$issue" ] && ! [[ "$issue" =~ ^[0-9]+$ ]]; then
        echo "Error: issue must be a number, got '${issue}'"
        exit 1
    fi

    echo "Preparing release for v${version}"

    check_prerequisites
    verify_clean_worktree
    fetch_from_official_remote
    check_due_feature_gates "$version"

    local branch_name
    branch_name=$(create_release_branch "$version")

    update_ui_submodule "$version"
    update_changelog "$version"
    rotate_release_managers "$version"
    commit_changes "$version"
    push_branch "$branch_name"
    create_pull_request "$version" "$issue"

    echo "Done. Review and merge the PR, then follow the instructions in the PR description."
}

main "$@"
