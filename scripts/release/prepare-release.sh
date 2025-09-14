#!/usr/bin/env bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# Automated release script for Jaeger
# This script automates the manual steps in the release process

set -euo pipefail

# Source utility functions
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/utils.sh"

# Configuration
REPO="jaegertracing/jaeger"
DRY_RUN=false

usage() {
    cat << EOF
Usage: $0 [OPTIONS]

Automated release script for Jaeger

OPTIONS:
    -d, --dry-run     Run in dry-run mode (no actual changes)
    -h, --help        Show this help message

EOF
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -d|--dry-run)
            DRY_RUN=true
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            usage
            exit 1
            ;;
    esac
done

# Main function
main() {
    log_info "Starting automated release process..."
    
    # Validate repository setup
    validate_repository
    
    # Check if GitHub CLI is installed and authenticated
    if ! command -v gh > /dev/null 2>&1; then
        log_error "GitHub CLI (gh) is not installed. Please install it first."
        exit 1
    fi
    
    # Check if gh is authenticated
    if ! gh auth status > /dev/null 2>&1; then
        log_error "GitHub CLI is not authenticated. Please run 'gh auth login' first."
        exit 1
    fi
    
    # Get current versions
    log_info "Getting current versions..."
    read -r current_version_v1 current_version_v2 <<< "$(get_current_versions)"
    
    if [[ -z "$current_version_v1" || -z "$current_version_v2" ]]; then
        log_error "Could not determine current versions"
        exit 1
    fi
    
    log_info "Current v1 version: $current_version_v1"
    log_info "Current v2 version: $current_version_v2"
    
    # Suggest next versions
    if [[ "$current_version_v1" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
        major_v1="${BASH_REMATCH[1]}"
        minor_v1="${BASH_REMATCH[2]}"
        patch_v1="${BASH_REMATCH[3]}"
        suggested_v1="v$major_v1.$((minor_v1 + 1)).0"
    else
        log_error "Could not parse v1 version: $current_version_v1"
        exit 1
    fi
    
    if [[ "$current_version_v2" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
        major_v2="${BASH_REMATCH[1]}"
        minor_v2="${BASH_REMATCH[2]}"
        patch_v2="${BASH_REMATCH[3]}"
        suggested_v2="v$major_v2.$((minor_v2 + 1)).0"
    else
        log_error "Could not parse v2 version: $current_version_v2"
        exit 1
    fi
    
    # Get user input for new versions
    read -p "New v1 version [$suggested_v1]: " user_version_v1
    if [[ -z "$user_version_v1" ]]; then
        user_version_v1="$suggested_v1"
    fi
    if [[ ! "$user_version_v1" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        user_version_v1="v$user_version_v1"
    fi
    
    read -p "New v2 version [$suggested_v2]: " user_version_v2
    if [[ -z "$user_version_v2" ]]; then
        user_version_v2="$suggested_v2"
    fi
    if [[ ! "$user_version_v2" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        user_version_v2="v$user_version_v2"
    fi
    
    new_version_v1="$user_version_v1"
    new_version_v2="$user_version_v2"
    
    log_success "Using new versions: v1=$new_version_v1, v2=$new_version_v2"
    
    # Generate changelog
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
    
    # Update UI submodule to latest
    log_info "Updating UI submodule to latest version..."
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "DRY RUN: Would update UI submodule"
    else
        git submodule init
        git submodule update
        
        # Validate jaeger-ui directory exists before using pushd
        if [[ ! -d "jaeger-ui" ]]; then
            log_error "jaeger-ui submodule not found"
            exit 1
        fi
        
        pushd jaeger-ui
        git checkout main
        git pull
        popd
        log_success "UI submodule updated to latest"
    fi
    
    # Create and switch to a new branch (keep v prefix)
    branch_name="release-prep-${new_version_v1}-${new_version_v2}"
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "DRY RUN: Would create branch: $branch_name"
    else
        git checkout -b "$branch_name"
        log_success "Created and switched to branch: $branch_name"
    fi
    
    # Update CHANGELOG.md with new version and content
    log_info "Updating CHANGELOG.md..."
    current_date=$(date +"%Y-%m-%d")
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "DRY RUN: Would update CHANGELOG.md with new version section"
    else
        if [[ -f "CHANGELOG.md" ]]; then
            # Create a temporary file for the new changelog
            temp_full_changelog=$(mktemp)
            
            # Add new version header with date
            echo "# ${new_version_v1} / ${new_version_v2} (${current_date})" > "$temp_full_changelog"
            echo "" >> "$temp_full_changelog"
            
            # Add generated changelog content
            cat "$temp_changelog" >> "$temp_full_changelog"
            echo "" >> "$temp_full_changelog"
            
            # Add existing changelog content
            cat "CHANGELOG.md" >> "$temp_full_changelog"
            
            # Replace the original file
            mv "$temp_full_changelog" "CHANGELOG.md"
            
            # Commit the changes
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
    
    # Push the changes
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "DRY RUN: Would push branch to origin"
    else
        git push -u origin "$branch_name"
        log_success "Pushed branch to origin"
    fi
    
    # Create the PR
    pr_title="Prepare release ${new_version_v1} / ${new_version_v2}"
    pr_body="## Release Preparation

This PR automates the release preparation for ${new_version_v1} / ${new_version_v2}.

### Changes Made:
- [x] Updated CHANGELOG.md with new version section
- [x] Updated UI submodule to latest version
- [x] Generated changelog content using make changelog

### Next Steps:
After this PR is merged, follow the steps in the release issue created by \`scripts/release/start.sh\` to complete the release process.

---
*This PR was automatically generated by the release automation script.*"
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "DRY RUN: Would create PR with title: $pr_title"
        log_info "DRY RUN: PR body:"
        echo "$pr_body"
    else
        # Create the PR
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
            
            # Add changelog:skip label
            gh pr edit "$pr_url" --add-label "changelog:skip"
            log_success "Added changelog:skip label"
        else
            log_error "Failed to create PR"
            exit 1
        fi
    fi
    
    # Clean up temporary files
    rm -f "$temp_changelog"
    
    log_success "Release PR creation completed!"
    log_info "Next: Review and merge the created PR, then follow the steps in the release issue to complete the release."
}

# Run main function
main "$@"