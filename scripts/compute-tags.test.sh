#!/bin/bash

# This script uses https://github.com/kward/shunit2 to run unit tests.
# The path to this repo must be provided via SHUNIT2 env var.

SHUNIT2="${SHUNIT2:?'expecting SHUNIT2 env var pointing to a dir with https://github.com/kward/shunit2 clone'}"

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

out=""
expect() {
    echo '   Actual:' "$out"
    while [ "$#" -gt 0 ]; do
        echo '   checking includes' "$1"
        assertContains "actual [$out]" "$out" "--tag docker.io/$1"
        assertContains "actual [$out]" "$out" "--tag quay.io/$1"
        shift
    done
}

expectNot() {
    echo '   Actual:' "$out"
    while [ "$#" -gt 0 ]; do
        echo '   checking excludes' "$1"
        assertNotContains "actual [$out]" "$out" "--tag docker.io/$1"
        assertNotContains "actual [$out]" "$out" "--tag quay.io/$1"
        shift
    done
}

testRandomBranch() {
    out=$(BRANCH=branch GITHUB_SHA=sha bash "$computeTags" foo/bar)
    expected=(
        "foo/bar"
        "foo/bar:latest"
        "foo/bar-snapshot:sha"
        "foo/bar-snapshot:latest"
    )
    expect "${expected[@]}"
}

testMainBranch() {
    out=$(BRANCH=main GITHUB_SHA=sha bash "$computeTags" foo/bar)
    expected=(
        "foo/bar"
        "foo/bar-snapshot:sha"
        "foo/bar-snapshot:latest"
    )
    expect "${expected[@]}"
    expectNot "foo/bar:latest"
}

testSemVerBranch() {
    out=$(BRANCH=v1.2.3 GITHUB_SHA=sha bash "$computeTags" foo/bar)
    expected=(
        "foo/bar"
        "foo/bar:latest"
        "foo/bar:1"
        "foo/bar:1.2"
        "foo/bar:1.2.3"
        "foo/bar-snapshot:sha"
        "foo/bar-snapshot:latest"
    )
    expect "${expected[@]}"
}

# shellcheck disable=SC1091
source "${SHUNIT2}/shunit2"
