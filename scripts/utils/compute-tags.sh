#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# Compute major/minor/etc image tags based on the current branch
# Milestone 1 change: Default to v2 tags unless VERSION is explicitly set to 1.
# Support --include-legacy-v1 to append v1 tags after v2 tags.

set -ef -o pipefail

# Set QUIET default before enabling set -u
QUIET=${QUIET:-}

if [[ -z $QUIET ]]; then
  set -x
fi

set -u

# Parse arguments
VERSION=""
BASE_BUILD_IMAGE=""
INCLUDE_LEGACY_V1=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      shift
      VERSION="$1"
      shift
      ;;
    --branch)
      shift
      BRANCH="$1"
      shift
      ;;
    --include-legacy-v1)
      INCLUDE_LEGACY_V1=1
      shift
      ;;
    *)
      # Positional argument - the image name
      if [[ -z "${BASE_BUILD_IMAGE}" ]]; then
        BASE_BUILD_IMAGE="$1"
      fi
      shift
      ;;
  esac
done

# Set defaults
if [[ -z "${VERSION}" ]]; then
  VERSION="2"
fi

BASE_BUILD_IMAGE=${BASE_BUILD_IMAGE:?'expecting Docker image name as argument, such as jaegertracing/jaeger'}
BRANCH=${BRANCH:?'expecting BRANCH env var'}
GITHUB_SHA=${GITHUB_SHA:-$(git rev-parse HEAD)}

# NOTE: VERSION and INCLUDE_LEGACY_V1 parameters are parsed and available for future use.
# Current tag generation does not differentiate between v1 and v2 image names.
# When v1/v2 image differentiation is implemented, this script can use these parameters
# to generate version-specific tags.

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
if [[ $BRANCH =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-rc[0-9]+)?$ ]]; then
    MAJOR_MINOR_PATCH=${BRANCH#v}
    tags "${BASE_BUILD_IMAGE}:${MAJOR_MINOR_PATCH}"
    tags "${BASE_BUILD_IMAGE}:latest"
elif [[ $BRANCH != "main" ]]; then
    # not on release tag nor on main - no tags are needed since we won't publish
    echo ""
    exit
fi

tags "${BASE_BUILD_IMAGE}-snapshot:${GITHUB_SHA}"
tags "${BASE_BUILD_IMAGE}-snapshot:latest"

echo "${IMAGE_TAGS}"
