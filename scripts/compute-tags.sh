#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# Compute major/minor/etc image tags based on the current branch

set -ef -o pipefail

if [[ -z $QUIET ]]; then
  set -x
fi

set -u

BASE_BUILD_IMAGE=${1:?'expecting Docker image name as argument, such as jaegertracing/jaeger'}
BRANCH=${BRANCH:?'expecting BRANCH env var'}
GITHUB_SHA=${GITHUB_SHA:?'expecting GITHUB_SHA env var'}
# allow substituting for ggrep on Mac, since its default grep doesn't grok -P flag.
GREP=${GREP:-"grep"}

# accumulate output in this variable
IMAGE_TAGS=""

# append given tag for docker.io and quay.io
tags() {
    if [[ -n "$IMAGE_TAGS" ]]; then
        # append space
        IMAGE_TAGS="${IMAGE_TAGS} "
    fi
    IMAGE_TAGS="${IMAGE_TAGS}--tag docker.io/${1} --tag quay.io/${1}"
}

## If we are on a release tag, let's extract the version number.
## The other possible values are 'main' or another branch name.
if [[ $BRANCH =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    MAJOR_MINOR_PATCH=$(echo "${BRANCH}" | ${GREP} -Po "([\d\.]+)")
    MAJOR_MINOR=$(echo "${MAJOR_MINOR_PATCH}" | awk -F. '{print $1"."$2}')
    MAJOR=$(echo "${MAJOR_MINOR_PATCH}" | awk -F. '{print $1}')

    tags "${BASE_BUILD_IMAGE}:${MAJOR_MINOR_PATCH}"
    tags "${BASE_BUILD_IMAGE}:${MAJOR_MINOR}"
    tags "${BASE_BUILD_IMAGE}:${MAJOR}"
    tags "${BASE_BUILD_IMAGE}:latest"
elif [[ $BRANCH != "main" ]]; then
    tags "${BASE_BUILD_IMAGE}:latest"
fi

tags "${BASE_BUILD_IMAGE}-snapshot:${GITHUB_SHA}"
tags "${BASE_BUILD_IMAGE}-snapshot:latest"

echo "${IMAGE_TAGS}"
