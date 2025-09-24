#!/usr/bin/env bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# Utility functions for release scripts

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
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


# Validate GitHub CLI availability and auth
validate_gh_cli() {
    if ! command -v gh > /dev/null 2>&1; then
        log_error "GitHub CLI (gh) is not installed. Please install it first."
        exit 1
    fi
    if ! gh auth status > /dev/null 2>&1; then
        log_error "GitHub CLI is not authenticated. Please run 'gh auth login' first."
        exit 1
    fi
    log_success "GitHub CLI validation passed"
}


# Validate repository setup
validate_repository() {
    log_info "Validating repository setup..."
    
    # Check if we're in a git repository
    if ! git rev-parse --git-dir > /dev/null 2>&1; then
        log_error "Not in a git repository"
        exit 1
    fi
    
    # Check for uncommitted changes
    if ! git diff-index --quiet HEAD --; then
        log_error "Uncommitted changes detected. Please commit or stash them before running the release process."
        exit 1
    fi
    
    # Check if we're on main branch
    current_branch=$(git branch --show-current)
    if [[ "$current_branch" != "main" ]]; then
        log_error "Not on main branch (current: $current_branch). Please switch to main branch."
        exit 1
    fi
    
    # Check remotes
    if ! git remote get-url upstream > /dev/null 2>&1; then
        log_error "No 'upstream' remote found. Please add upstream remote pointing to jaegertracing/jaeger"
        exit 1
    fi
    
    if ! git remote get-url origin > /dev/null 2>&1; then
        log_error "No 'origin' remote found. Please add origin remote pointing to your fork"
        exit 1
    fi
    
    # Validate upstream points to official Jaeger repo
    upstream_url=$(git remote get-url upstream)
    if [[ "$upstream_url" != *"jaegertracing/jaeger"* ]]; then
        log_error "Upstream remote does not point to jaegertracing/jaeger: $upstream_url"
        exit 1
    fi
    
    # Validate origin points to a fork (not the official repo)
    origin_url=$(git remote get-url origin)
    if [[ "$origin_url" == *"jaegertracing/jaeger"* ]]; then
        log_error "Origin remote points to official repo. It should point to your fork: $origin_url"
        exit 1
    fi
    
    log_success "Repository setup validation passed"
}


# Get current versions
get_current_versions() {
    local v1_version v2_version
    
    # Try to get versions from make
    v1_version=$(make -s echo-v1 2>/dev/null || echo "")
    v2_version=$(make -s echo-v2 2>/dev/null || echo "")
    
    # Fallback to git tags if make fails
    if [[ -z "$v1_version" ]]; then
        v1_version=$(git tag --list "v1.*" --sort=-version:refname | head -n 1)
    fi
    
    if [[ -z "$v2_version" ]]; then
        v2_version=$(git tag --list "v2.*" --sort=-version:refname | head -n 1)
    fi
    
    echo "$v1_version $v2_version"
}
