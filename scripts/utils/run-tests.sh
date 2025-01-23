#!/bin/bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

UTILS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$UTILS_DIR/../.."

# Define list of test files explicitly here , to be dynamic to the location of the test file
TEST_FILES=(
    "$UTILS_DIR/compute-tags.test.sh"
    "$REPO_ROOT/plugin/storage/cassandra/schema/create.test.sh"
)

run_test_file() {
    local test_file="$1"
    if [ ! -f "$test_file" ]; then
        echo "Error: Test file not found: $test_file"
        return 1
    fi
    
    echo "Running tests from: $test_file"
    
    export SHUNIT2="${SHUNIT2:?'SHUNIT2 environment variable must be set'}"

    bash "$test_file"
    local result=$?
    echo "Test file $test_file completed with status: $result"
    return $result
}

main() {

    if [ ! -f "${SHUNIT2}/shunit2" ]; then
        echo "Error: shunit2 not found at ${SHUNIT2}/shunit2"
        exit 1
    fi
    local failed=0
    local total=0
    local passed=0
    local failed_tests=()

    # Run all test files
    for test_file in "${TEST_FILES[@]}"; do
        ((total++))
        if ! run_test_file "$test_file"; then
            failed=1
            failed_tests+=("$test_file")
        else
            ((passed++))
        fi
    done

    echo "-------------------"
    echo "Test Summary:"
    echo "Total: $total"
    echo "Passed: $passed"
    echo "Failed: $((total - passed))"
    
    if [ ${#failed_tests[@]} -gt 0 ]; then
        echo "Failed tests:"
        for test in "${failed_tests[@]}"; do
            echo "  - $(basename "$test")"
        fi
    fi

    exit $failed
}

main