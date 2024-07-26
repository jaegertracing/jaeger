#!/bin/bash

computeTags=$(dirname $0)/compute-tags.sh

# suppress command echoing by compute-tags.sh
export QUIET=1

testRequireImageName() {
    err=$(bash $computeTags 2>&1)
    assertContains "$err" 'expecting Docker image name'
}

testRequireBranch() {
    err=$(bash $computeTags foo/bar 2>&1)
    assertContains "$err" "$err" 'expecting BRANCH env var'
}

testRequireGithubSha() {
    err=$(BRANCH=abcd bash $computeTags foo/bar 2>&1)
    assertContains "$err" "$err" 'expecting GITHUB_SHA env var'
}

out=""
expect() {
    echo '   Actual:' $out
    while [ "$#" -gt 0 ]; do
        echo '   checking' $1
        assertContains "actual !!$out!!" "$out" "--tag docker.io/$1"
        assertContains "actual !!$out!!" "$out" "--tag quay.io/$1"
        shift
    done
}

testRandomBranch() {
    out=$(BRANCH=branch GITHUB_SHA=sha bash $computeTags foo/bar)
    expected=(
        "foo/bar"
        "foo/bar:latest"
        "foo/bar-snapshot:sha"
        "foo/bar-snapshot:latest"
    )
    expect "${expected[@]}"
}

testMainBranch() {
    out=$(BRANCH=main GITHUB_SHA=sha bash $computeTags foo/bar)
    # TODO we do not want :latest tag in this scenario for non-snapshot images
    expected=(
        "foo/bar"
        "foo/bar:latest"
        "foo/bar-snapshot:sha"
        "foo/bar-snapshot:latest"
    )
    expect "${expected[@]}"
}

testSemVerBranch() {
    out=$(BRANCH=v1.2.3 GITHUB_SHA=sha bash $computeTags foo/bar)
    # TODO we want :latest tag in this scenario, it's currently not produced
    expected=(
        "foo/bar"
        "foo/bar:1"
        "foo/bar:1.2"
        "foo/bar:1.2.3"
        "foo/bar-snapshot:sha"
        "foo/bar-snapshot:latest"
    )
    expect "${expected[@]}"
}

source ${SHUNIT2:?'expecting SHUNIT2 env var pointing to a dir with https://github.com/kward/shunit2 clone'}/shunit2
