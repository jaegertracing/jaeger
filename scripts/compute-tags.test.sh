#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# This script uses https://github.com/kward/shunit2 to run unit tests.
# The path to this repo must be provided via SHUNIT2 env var.

SHUNIT2="${SHUNIT2:?'expecting SHUNIT2 env var pointing to a dir with https://github.com/kward/shunit2 clone'}"

# if running on MacOS, `brew install grep` and run with GREP=ggrep
GREP=${GREP:-grep}

# shellcheck disable=SC2086
computeTags="$(dirname $0)/compute-tags.sh"

# suppress command echoing by compute-tags.sh
export QUIET=1

# unset env vars that were possibly set by the caller, since we test against them
unset BRANCH
unset GITHUB_SHA

testRequireImageName() {
    err=$(bash "$computeTags" 2>&1)
    assertContains "$err" 'expecting Docker image name'
}

testRequireBranch() {
    err=$(GITHUB_SHA=sha bash "$computeTags" foo/bar 2>&1)
    assertContains "$err" "$err" 'expecting BRANCH env var'
}

testRequireGithubSha() {
    err=$(BRANCH=abcd bash "$computeTags" foo/bar 2>&1)
    assertContains "$err" "$err" 'expecting GITHUB_SHA env var'
}

# out is global var which is populated for every output under test
out=""

scan_list() {
    local target="$1"
    echo "$out" | tr ' ' '\n' | $GREP -v '^--tag$' | $GREP -Po "^($target)"'$'
}

expect_contains() {
    local target="$1"
    # shellcheck disable=SC2155
    local found=$(scan_list "$target")
    assertContains "$found" "$target"
}

expect_not_contains() {
    local target="$1"
    # shellcheck disable=SC2155
    local found=$(scan_list "$target")
    assertNotContains "$found" "$target"
}

expect() {
    echo '   Actual:' "$out"
    while [ "$#" -gt 0 ]; do
        echo '   checking includes' "$1"
        expect_contains "docker.io/$1"
        expect_contains "quay.io/$1"
        shift
    done
}

expect_not() {
    echo '   Actual:' "$out"
    while [ "$#" -gt 0 ]; do
        echo '   checking excludes' "$1"
        expect_not_contains "docker.io/$1"
        expect_not_contains "quay.io/$1"
        shift
    done
}

testRandomBranch() {
    out=$(BRANCH=branch GITHUB_SHA=sha bash "$computeTags" foo/bar)
    expected=(
        "foo/bar:latest"
        "foo/bar-snapshot:sha"
        "foo/bar-snapshot:latest"
    )
    expect "${expected[@]}"
    expect_not "foo/bar"
}

testMainBranch() {
    out=$(BRANCH=main GITHUB_SHA=sha bash "$computeTags" foo/bar)
    expected=(
        "foo/bar-snapshot:sha"
        "foo/bar-snapshot:latest"
    )
    expect "${expected[@]}"
    expect_not "foo/bar" "foo/bar:latest"
}

testSemVerBranch() {
    out=$(BRANCH=v1.2.3 GITHUB_SHA=sha bash "$computeTags" foo/bar)
    expected=(
        "foo/bar:latest"
        "foo/bar:1"
        "foo/bar:1.2"
        "foo/bar:1.2.3"
        "foo/bar-snapshot:sha"
        "foo/bar-snapshot:latest"
    )
    expect "${expected[@]}"
    expect_not "foo/bar"
}

# shellcheck disable=SC1091
source "${SHUNIT2}/shunit2"
