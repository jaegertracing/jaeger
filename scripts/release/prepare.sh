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
AUTO_TAG=false

usage() {
    cat << EOF
Usage: $0 [OPTIONS] <v1_version> <v2_version>

Automated release script for Jaeger

OPTIONS:
    -d, --dry-run     Run in dry-run mode (no actual changes)
    --auto-tag        Create and push tags automatically after PR creation
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
        --auto-tag)
            AUTO_TAG=true
            shift
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
    validate_repository
    git fetch upstream
    git checkout main
    git pull --ff-only upstream main
}

validate_environment() {
    validate_gh_cli
}

determine_current_versions() {
    current_versions=($(get_current_versions))
    current_v1="${current_versions[0]}"
    current_v2="${current_versions[1]}"
}

validate_semver_increment() {
    local new_version="$1"
    local current_version="$2"
    # Expect format vX.Y.Z
    if ! [[ "$new_version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        log_error "Version $new_version does not follow semantic versioning format (vX.Y.Z)"
        return 1
    fi
    # Ensure new_version > current_version
    local highest
    highest=$(printf "%s\n" "$current_version" "$new_version" | sort -V | tail -n1)
    if [[ "$highest" != "$new_version" ]]; then
        log_error "New version $new_version must be greater than current version $current_version"
        return 1
    fi
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

### ✨ New Features
- Automated release process implementation

### 🐛 Bug Fixes
- Various bug fixes and improvements

### 📚 Documentation
- Updated release documentation

EOF
    fi
}

update_ui_submodule() {
    log_info "Updating UI submodule to latest version..."
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "DRY RUN: Would update UI submodule"
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
        log_info "DRY RUN: Would create branch: $branch_name"
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
        log_info "DRY RUN: Would push branch to origin"
    else
        git push -u origin "$branch_name"
        log_success "Pushed branch to origin"
    fi
}

open_pr_with_body() {
    pr_title="Prepare release ${new_version_v1} / ${new_version_v2}"
    changelog_block=$(cat "$temp_changelog")
    max_lines=400
    if [ "$(printf "%s\n" "$changelog_block" | wc -l)" -gt "$max_lines" ]; then
        log_warning "Changelog exceeds $max_lines lines and will be truncated in the PR description."
        changelog_block="$(printf "%s\n" "$changelog_block" | head -n "$max_lines")\n...\n(truncated)"
    fi
    pr_body="## Release Preparation

This PR automates the release preparation for ${new_version_v1} / ${new_version_v2}.

### Changes Made:
- [x] Updated CHANGELOG.md with new version section
- [x] Updated UI submodule to latest version
- [x] Generated changelog content using make changelog

### Changelog Content
\`\`\`
${changelog_block}
\`\`\`

### Next Steps:
After this PR is merged, follow the steps in the release issue created by \`scripts/release/start.sh\` to complete the release process.

---
*This PR was automatically generated by the release automation script.*"
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "DRY RUN: Would create PR with title: $pr_title"
        log_info "DRY RUN: PR body:"
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
        log_info "DRY RUN: Would create and push tags ${new_version_v1} ${new_version_v2}"
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
    initialize_and_update_main
    validate_environment
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

    if [[ "$AUTO_TAG" == "true" ]]; then
        log_info "Auto-tagging is enabled. Creating tags now."
        create_release_tags
    else
        log_info "Run the following commands to create and push tags after merging the PR:"
        print_tag_commands
    fi
}

# Run main function
main "$@"


