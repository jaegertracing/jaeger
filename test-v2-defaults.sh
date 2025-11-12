#!/bin/bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# Test script to verify Milestone 1: v2 default reassignment
# This script verifies that build targets default to v2 except for the four exception targets.

set -uo pipefail

cd "$(dirname "$0")"

echo "Testing Milestone 1: v2 Default Reassignment"
echo "=============================================="
echo ""

# Color codes for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

pass_count=0
fail_count=0

test_version() {
    local target="$1"
    local expected_version="$2"
    local description="$3"
    
    echo -n "Testing $description... "
    
    # Run make with -n (dry-run) and grep for latestVersion
    local actual_version
    actual_version=$(make -n "$target" GOOS=linux GOARCH=amd64 2>&1 | grep -oP 'latestVersion=v\K[0-9]+\.[0-9]+\.[0-9]+' | head -1 || true)
    
    if [ -z "$actual_version" ]; then
        echo -e "${RED}FAIL${NC} (no version found)"
        ((fail_count++))
        return 0
    fi
    
    local expected_major="${expected_version%%.*}"
    local actual_major="${actual_version%%.*}"
    
    if [ "$actual_major" = "$expected_major" ]; then
        echo -e "${GREEN}PASS${NC} (v$actual_version)"
        ((pass_count++))
    else
        echo -e "${RED}FAIL${NC} (expected v$expected_version, got v$actual_version)"
        ((fail_count++))
    fi
    return 0
}

test_override() {
    local target="$1"
    local override_version="$2"
    local expected_version="$3"
    local description="$4"
    
    echo -n "Testing $description... "
    
    # Run make with -n (dry-run) and grep for latestVersion
    local actual_version
    actual_version=$(JAEGER_VERSION="$override_version" make -n "$target" GOOS=linux GOARCH=amd64 2>&1 | grep -oP 'latestVersion=v\K[0-9]+\.[0-9]+\.[0-9]+' | head -1 || true)
    
    if [ -z "$actual_version" ]; then
        echo -e "${RED}FAIL${NC} (no version found)"
        ((fail_count++))
        return 0
    fi
    
    local expected_major="${expected_version%%.*}"
    local actual_major="${actual_version%%.*}"
    
    if [ "$actual_major" = "$expected_major" ]; then
        echo -e "${GREEN}PASS${NC} (v$actual_version)"
        ((pass_count++))
    else
        echo -e "${RED}FAIL${NC} (expected v$expected_version, got v$actual_version)"
        ((fail_count++))
    fi
    return 0
}

echo "1. Testing v2 defaults for non-exception targets:"
echo "--------------------------------------------------"
test_version "build-jaeger" "2" "build-jaeger defaults to v2"
test_version "build-tracegen" "2" "build-tracegen defaults to v2"
test_version "build-anonymizer" "2" "build-anonymizer defaults to v2"
test_version "build-esmapping-generator" "2" "build-esmapping-generator defaults to v2"
test_version "build-es-index-cleaner" "2" "build-es-index-cleaner defaults to v2"
test_version "build-es-rollover" "2" "build-es-rollover defaults to v2"
test_version "build-remote-storage" "2" "build-remote-storage defaults to v2"
echo ""

echo "2. Testing v1 defaults for exception targets:"
echo "----------------------------------------------"
test_version "build-all-in-one" "1" "build-all-in-one defaults to v1"
test_version "build-query" "1" "build-query defaults to v1"
test_version "build-collector" "1" "build-collector defaults to v1"
test_version "build-ingester" "1" "build-ingester defaults to v1"
echo ""

echo "3. Testing override mechanism (v2 targets to v1):"
echo "--------------------------------------------------"
test_override "build-jaeger" "1" "1" "JAEGER_VERSION=1 build-jaeger uses v1"
test_override "build-tracegen" "1" "1" "JAEGER_VERSION=1 build-tracegen uses v1"
echo ""

echo "4. Testing override mechanism (v1 targets to v2):"
echo "--------------------------------------------------"
test_override "build-all-in-one" "2" "2" "JAEGER_VERSION=2 build-all-in-one uses v2"
test_override "build-collector" "2" "2" "JAEGER_VERSION=2 build-collector uses v2"
echo ""

echo "=============================================="
echo "Test Results:"
echo "  Passed: $pass_count"
echo "  Failed: $fail_count"
echo "=============================================="

if [ "$fail_count" -eq 0 ]; then
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}Some tests failed!${NC}"
    exit 1
fi
