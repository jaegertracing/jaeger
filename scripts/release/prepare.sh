#!/usr/bin/env bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# Automated release preparation script (argument-driven)

set -euo pipefail

# Source utility functions
source "$(dirname $0)/utils.sh"

# Configuration
REPO="jaegertracing/jaeger"
DRY_RUN=false
TRACKING_ISSUE=""

usage() {
    cat << EOF
Usage: $0 [OPTIONS] <v1_version> <v2_version>

Automated release script for Jaeger

OPTIONS:
    -d, --dry-run     Run in dry-run mode (no actual changes)
    --tracking-issue  Link to the main tracking issue (e.g., #1234)
    -h, --help        Show this help message

EOF
}

# Parse command line arguments
POSITIONAL=()
while [[ $# -gt 0 ]]; do
    case $1 in
        -d|--dry-run)
            DRY_RUN=true
            shift
            ;;
        --tracking-issue)
            TRACKING_ISSUE="$2"
            shift 2
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            POSITIONAL+=("$1")
            shift
            ;;
    esac
done
set -- "${POSITIONAL[@]}"

# Expect explicit versions as positional args (provided by start.sh substitutions)
if [[ $# -ne 2 ]]; then
    log_error "Expected two arguments: <v1_version> <v2_version>"
    usage
    exit 1
fi
new_version_v1="$1"
new_version_v2="$2"

initialize_and_update_main() {
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "DRY RUN: Skipping repository validation and git sync"
        return 0
    fi
    validate_repository
    git fetch upstream
    git checkout main
    git pull --ff-only upstream main
}

validate_environment() {
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "DRY RUN: Skipping GitHub CLI validation"
        return 0
    fi
    validate_gh_cli
}


determine_current_versions() {
    read -r current_v1 current_v2 <<< "$(get_current_versions)"
    log_info "Current versions: $current_v1 / $current_v2"
}


validate_semver_increment() {
    local new_version="$1"
    local current_version="$2"
    # Expect format vX.Y.Z
    if ! [[ "$new_version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        log_error "Version $new_version does not follow semantic versioning format (vX.Y.Z)"
        return 1
    fi
    # No monotonicity check; start.sh is the source of truth
    return 0
}

normalize_and_validate_versions() {
    new_version_v1=$(echo "$new_version_v1" | sed 's/^v//')
    new_version_v1="v$new_version_v1"
    new_version_v2=$(echo "$new_version_v2" | sed 's/^v//')
    new_version_v2="v$new_version_v2"
    validate_semver_increment "$new_version_v1" "$current_v1" || exit 1
    validate_semver_increment "$new_version_v2" "$current_v2" || exit 1
}

generate_changelog() {
    log_info "Generating changelog..."
    temp_changelog=$(mktemp)
    trap "rm -f \"$temp_changelog\"" EXIT
    if make changelog > "$temp_changelog" 2>/dev/null; then
        log_success "Changelog generated successfully"
    else
        log_warning "make changelog failed, using fallback template"
        cat > "$temp_changelog" << EOF
## Changes since last release

### âœ¨ New Features
- Automated release process implementation

### ðŸ› Bug Fixes
- Various bug fixes and improvements

### ðŸ“š Documentation
- Updated release documentation

EOF
    fi
}

update_ui_submodule() {
    log_info "Updating UI submodule to latest version..."
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "DRY RUN: Skipping UI submodule update"
    else
        git submodule init
        git submodule update
        if [[ ! -d "jaeger-ui" ]]; then
            log_error "jaeger-ui submodule not found"
            exit 1
        fi
        pushd jaeger-ui || { log_error "Failed to enter jaeger-ui directory"; exit 1; }
        git checkout main || { log_error "Failed to checkout main branch in UI submodule"; popd; exit 1; }
        git pull || { log_error "Failed to pull latest changes in UI submodule"; popd; exit 1; }
        popd
        log_success "UI submodule updated to latest"
    fi
}

create_release_branch() {
    branch_name="release-prep-${new_version_v1}-${new_version_v2}"
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "DRY RUN: Skipping branch creation: $branch_name"
    else
        git checkout -b "$branch_name"
        log_success "Created and switched to branch: $branch_name"
    fi
}

update_changelog_and_commit() {
    log_info "Updating CHANGELOG.md..."
    current_date=$(date +"%Y-%m-%d")
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "DRY RUN: Would update CHANGELOG.md with new version section"
    else
        if [[ -f "CHANGELOG.md" ]]; then
            temp_full_changelog=$(mktemp)
            trap "rm -f \"$temp_full_changelog\"" EXIT
            echo "# ${new_version_v1} / ${new_version_v2} (${current_date})" > "$temp_full_changelog"
            echo "" >> "$temp_full_changelog"
            cat "$temp_changelog" >> "$temp_full_changelog"
            echo "" >> "$temp_full_changelog"
            cat "CHANGELOG.md" >> "$temp_full_changelog"
            mv "$temp_full_changelog" "CHANGELOG.md"
            git add "CHANGELOG.md"
            git add "jaeger-ui"
            git commit -m "Prepare release ${new_version_v1} / ${new_version_v2}

- Updated CHANGELOG.md with new version section
- Updated UI submodule to latest version
- Generated changelog content using make changelog"
            log_success "CHANGELOG.md and UI submodule updated and committed"
        else
            log_error "CHANGELOG.md not found"
            exit 1
        fi
    fi
}

push_branch() {
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "DRY RUN: Skipping push of branch to origin"
    else
        git push -u origin "$branch_name"
        log_success "Pushed branch to origin"
    fi
}

open_pr_with_body() {
    pr_title="Prepare release ${new_version_v1} / ${new_version_v2}"
    pr_body="Prepare release ${new_version_v1} / ${new_version_v2}.

This PR updates CHANGELOG.md with a new version section and bumps UI submodule."
    if [[ -n "$TRACKING_ISSUE" ]]; then
        pr_body+=$'\n\nPart of release tracking issue #'"$TRACKING_ISSUE"''
    fi
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "DRY RUN: Skipping PR creation. Preview below:"
        echo "Title: $pr_title"
        echo "$pr_body"
    else
        pr_output=$(gh pr create \
            --repo "$REPO" \
            --title "$pr_title" \
            --body "$pr_body" \
            --base main \
            --head "$branch_name")
        pr_exit_code=$?
        pr_url=$(echo "$pr_output" | tail -n 1)
        if [[ $pr_exit_code -eq 0 ]]; then
            log_success "Release PR created: $pr_url"
            gh pr edit "$pr_url" --add-label "changelog:skip"
            log_success "Added changelog:skip label"
        else
            log_error "Failed to create PR"
            exit 1
        fi
    fi
}

print_tag_commands() {
    cat << EOF
git checkout main
git pull
git tag ${new_version_v1} -s   # sign the v1 tag
git tag ${new_version_v2} -s   # sign the v2 tag
git push upstream ${new_version_v1} ${new_version_v2}
EOF
}

create_release_tags() {
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "DRY RUN: Skipping tag creation and push for ${new_version_v1} ${new_version_v2}"
        return 0
    fi
    git checkout main
    git pull --ff-only upstream main || git pull
    git tag "${new_version_v1}" -s -m "${new_version_v1}"
    git tag "${new_version_v2}" -s -m "${new_version_v2}"
    git push upstream "${new_version_v1}" "${new_version_v2}"
}

# Main function
main() {
    log_info "Starting automated release process..."
    if [[ "$DRY_RUN" != "true" ]]; then
        initialize_and_update_main
        validate_environment
    else
        log_info "DRY RUN: Skipping environment and repository setup"
    fi
    determine_current_versions
    normalize_and_validate_versions
    generate_changelog
    update_ui_submodule
    create_release_branch
    update_changelog_and_commit
    push_branch
    open_pr_with_body
    log_success "Release PR creation completed!"
    log_info "Next: Review and merge the created PR, then follow the steps in the release issue to complete the release."

    log_info "Tag creation is documented in the tracking issue; no tagging performed here."
}

# Run main function
main "$@"


