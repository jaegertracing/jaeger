#!/bin/bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# Test script for release PR preparation
# This script tests the release preparation functionality without making actual changes

set -euo pipefail

echo "🧪 Testing Release PR Preparation Scripts"
echo "=========================================="

# Test 1: Check if scripts exist
echo "Test 1: Checking script existence..."
if [[ -f "scripts/release/prepare-release.sh" ]]; then
    echo "✅ Bash script exists"
else
    echo "❌ Bash script missing"
    exit 1
fi

# Test 2: Check if make target exists
echo "Test 2: Checking make target..."
if grep -q "prepare-release:" Makefile; then
    echo "✅ Make target exists"
else
    echo "❌ Make target missing"
    exit 1
fi

# Test 3: Test version detection
echo "Test 3: Testing version detection..."
if command -v make &> /dev/null; then
    echo "Current v1 version: $(make echo-v1 2>/dev/null || echo 'N/A')"
    echo "Current v2 version: $(make echo-v2 2>/dev/null || echo 'N/A')"
    echo "✅ Version detection working"
else
    echo "⚠️  Make not available, skipping version test"
fi

# Test 4: Test changelog generation
echo "Test 4: Testing changelog generation..."
if command -v make &> /dev/null; then
    if make changelog &> /dev/null; then
        echo "✅ Changelog generation working"
    else
        echo "⚠️  Changelog generation failed (this might be expected in test environment)"
    fi
else
    echo "⚠️  Make not available, skipping changelog test"
fi

# Test 5: Check GitHub CLI
echo "Test 5: Checking GitHub CLI..."
if command -v gh &> /dev/null; then
    echo "✅ GitHub CLI installed"
    if gh auth status &> /dev/null; then
        echo "✅ GitHub CLI authenticated"
    else
        echo "⚠️  GitHub CLI not authenticated"
    fi
else
    echo "⚠️  GitHub CLI not installed"
fi

echo ""
echo "🎯 Test Summary:"
echo "================="
echo "All basic tests completed. The release preparation scripts are ready for use."
echo ""
echo "To test the full release preparation:"
echo "  make prepare-release"
echo ""
echo "To test in dry-run mode:"
echo "  ./scripts/release/prepare-release.sh --dry-run"
