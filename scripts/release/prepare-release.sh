#!/usr/bin/env bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# Automated release script for Jaeger
# This script automates the manual steps in the release process

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
REPO="jaegertracing/jaeger"
DRY_RUN=false
AUTO_TAG=false

# Functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

usage() {
    cat << EOF
Usage: $0 [OPTIONS]

Automated release script for Jaeger

OPTIONS:
    -d, --dry-run     Run in dry-run mode (no actual changes)
    -t, --auto-tag    Automatically create and push tags
    -h, --help        Show this help message

EXAMPLES:
    $0                    # Interactive mode
    $0 --dry-run         # Test run without changes
    $0 --auto-tag        # Full automation including tags
EOF
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -d|--dry-run)
            DRY_RUN=true
            shift
            ;;
        -t|--auto-tag)
            AUTO_TAG=true
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

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    # Check if gh CLI is installed
    if ! command -v gh &> /dev/null; then
        log_error "GitHub CLI (gh) is not installed. Please install it first."
        exit 1
    fi
    
    # Check if gh is authenticated
    if ! gh auth status &> /dev/null; then
        log_error "GitHub CLI is not authenticated. Please run 'gh auth login' first."
        exit 1
    fi
    
    # Check if we're in a git repository
    if ! git rev-parse --git-dir > /dev/null 2>&1; then
        log_error "Not in a git repository. Please run this script from the Jaeger repository root."
        exit 1
    fi
    
    # Check if we're on main branch
    if [[ $(git branch --show-current) != "main" ]]; then
        log_warning "Not on main branch. Current branch: $(git branch --show-current)"
        if [[ -t 0 ]]; then
            # Interactive mode - ask for confirmation
            read -p "Continue anyway? (y/N): " -n 1 -r
            echo
            if [[ ! $REPLY =~ ^[Yy]$ ]]; then
                exit 1
            fi
        else
            # Non-interactive mode - assume yes
            log_info "Non-interactive mode: continuing anyway"
        fi
    fi
    
    log_success "Prerequisites check passed"
}

# Get current versions
get_current_versions() {
    log_info "Getting current versions..."
    
    if ! current_version_v1=$(make -s "echo-v1" 2>/dev/null); then
        log_error "Failed to fetch current v1 version from make echo-v1"
        exit 1
    fi
    
    if ! current_version_v2=$(make -s "echo-v2" 2>/dev/null); then
        log_error "Failed to fetch current v2 version from make echo-v2"
        exit 1
    fi
    
    log_success "Current versions: v1=${current_version_v1}, v2=${current_version_v2}"
}

# Generate changelog
generate_changelog() {
    log_info "Generating changelog..."
    
    if ! changelog_output=$(make changelog 2>&1); then
        log_error "Failed to generate changelog: $changelog_output"
        exit 1
    fi
    
    log_success "Changelog generated successfully"
}

# Determine next versions
determine_next_versions() {
    log_info "Determining next versions..."
    
    # Parse current v1 version
    clean_version_v1="${current_version_v1#v}"
    IFS='.' read -r major_v1 minor_v1 patch_v1 <<< "$clean_version_v1"
    
    # Parse current v2 version
    clean_version_v2="${current_version_v2#v}"
    IFS='.' read -r major_v2 minor_v2 patch_v2 <<< "$clean_version_v2"
    
    # Suggest next versions (minor bump)
    suggested_v1="${major_v1}.$((minor_v1 + 1)).0"
    suggested_v2="${major_v2}.$((minor_v2 + 1)).0"
    
    echo "Current v1 version: ${current_version_v1}"
    read -r -e -p "New v1 version: " -i "${suggested_v1}" user_version_v1
    
    echo "Current v2 version: ${current_version_v2}"
    read -r -e -p "New v2 version: " -i "${suggested_v2}" user_version_v2
    
    new_version_v1="v${user_version_v1}"
    new_version_v2="v${user_version_v2}"
    
    log_success "Using new versions: v1=${new_version_v1}, v2=${new_version_v2}"
}

# Create release PR
create_release_pr() {
    log_info "Creating release PR..."
    
    # Create temporary changelog file
    temp_changelog=$(mktemp)
    make changelog > "$temp_changelog"
    
    # Update UI submodule to latest version
    log_info "Updating UI submodule to latest version..."
    if [ -d "jaeger-ui" ]; then
        pushd jaeger-ui > /dev/null
        git checkout main
        git pull origin main
        # Get the latest commit hash for the UI version
        ui_latest_commit=$(git rev-parse HEAD)
        ui_latest_tag=$(git describe --tags --abbrev=0 2>/dev/null || echo "latest")
        popd > /dev/null
        log_success "UI submodule updated to latest: ${ui_latest_tag} (${ui_latest_commit:0:8})"
    else
        log_warning "jaeger-ui submodule not found, skipping UI update"
    fi

    # Update CHANGELOG.md with new version header and generated content
    log_info "Updating CHANGELOG.md..."
    current_date=$(date +"%Y-%m-%d")
    if [ -f "CHANGELOG.md" ]; then
        temp_full_changelog=$(mktemp)
        # Header matches guideline: "1.x.x / 2.x.x (YYYY-MM-DD)"
        echo "${new_version_v1} / ${new_version_v2} (${current_date})" > "$temp_full_changelog"
        echo "" >> "$temp_full_changelog"
        cat "$temp_changelog" >> "$temp_full_changelog"
        echo "" >> "$temp_full_changelog"
        cat CHANGELOG.md >> "$temp_full_changelog"
        mv "$temp_full_changelog" CHANGELOG.md
        log_success "CHANGELOG.md updated"
    else
        log_error "CHANGELOG.md not found"
        rm -f "$temp_changelog"
        exit 1
    fi

    # Create PR title and body
    pr_title="Prepare release ${new_version_v1} / ${new_version_v2}"
    pr_body=$(cat << EOF
## Release Preparation

This PR automates the release preparation for ${new_version_v1} / ${new_version_v2}.

### Changes Made:
- [x] Updated CHANGELOG.md with new version section
- [x] Generated changelog content using \`make changelog\`
- [x] Updated UI submodule to latest version

### Next Steps:
1. Review and merge this PR
2. Update UI submodule to latest version
3. Create release tags: \`git tag ${new_version_v1} -s\` and \`git tag ${new_version_v2} -s\`
4. Push tags: \`git push upstream ${new_version_v1} ${new_version_v2}\`
5. Create GitHub release
6. Trigger CI release workflow

### Generated Changelog:
\`\`\`
$(cat "$temp_changelog")
\`\`\`

---
*This PR was automatically generated by the release automation script.*
EOF
)
    
    if $DRY_RUN; then
        log_info "DRY RUN: Would create PR with title: $pr_title"
        log_info "DRY RUN: PR body:"
        echo "$pr_body"
        log_info "DRY RUN: Would create branch, commit CHANGELOG.md, push, and open PR"
    else
        # Ensure we're on main branch before creating release branch
        current_branch=$(git branch --show-current)
        if [ "$current_branch" != "main" ]; then
            log_info "Switching to main branch..."
            git checkout main
            git pull origin main
        fi
        
        # Create and switch to a new branch
        branch_name="release-prep-${new_version_v1#v}-${new_version_v2#v}"
        git checkout -b "$branch_name"

        # Commit updated CHANGELOG.md and UI submodule changes
        git add CHANGELOG.md jaeger-ui
        git commit -m "Prepare release ${new_version_v1} / ${new_version_v2}"

        # Push branch
        git push -u origin "$branch_name"

        # Create the PR and robustly capture its URL
        pr_out_file=$(mktemp)
        if gh pr create \
            --repo "$REPO" \
            --title "$pr_title" \
            --body "$pr_body" \
            --base main \
            --head "$branch_name" | tee "$pr_out_file" >/dev/null; then
            pr_url=$(tr -d '\n' < "$pr_out_file")
            log_success "Release PR created: $pr_url"
            # Add changelog:skip label
            gh pr edit "$pr_url" --add-label "changelog:skip"
            log_success "Added changelog:skip label"
        else
            log_error "Failed to create PR"
            rm -f "$pr_out_file"
            rm -f "$temp_changelog"
            exit 1
        fi
        rm -f "$pr_out_file"
    fi
    
    # Clean up
    rm -f "$temp_changelog"
}

# Create and push tags
create_tags() {
    log_info "Creating and pushing tags..."
    
    if $DRY_RUN; then
        log_info "DRY RUN: Would execute the following commands:"
        echo "git tag ${new_version_v1} -s"
        echo "git tag ${new_version_v2} -s"
        echo "git push upstream ${new_version_v1} ${new_version_v2}"
        return
    fi
    
    if $AUTO_TAG; then
        log_info "Automatically creating and pushing tags..."
        
        # Create tags
        git tag "${new_version_v1}" -s
        git tag "${new_version_v2}" -s
        
        # Push tags
        git push upstream "${new_version_v1}" "${new_version_v2}"
        
        log_success "Tags created and pushed successfully"
    else
        log_info "Manual tag creation mode. Please execute the following commands:"
        echo
        echo "git tag ${new_version_v1} -s"
        echo "git tag ${new_version_v2} -s"
        echo "git push upstream ${new_version_v1} ${new_version_v2}"
        echo
        read -p "Press Enter after you've created and pushed the tags..."
    fi
}

# Main execution
main() {
    log_info "Starting automated release process..."
    
    check_prerequisites
    get_current_versions
    determine_next_versions
    generate_changelog
    create_release_pr
    
    log_info "Release PR creation completed!"
    
    if $AUTO_TAG; then
        create_tags
        log_success "Release automation completed successfully!"
    else
        log_info "To complete the release, please:"
        log_info "1. Review and merge the created PR"
        log_info "2. Update UI submodule if needed"
        log_info "3. Create and push tags manually"
        log_info "4. Create GitHub release"
        log_info "5. Trigger CI release workflow"
    fi
}

# Run main function
main "$@"
