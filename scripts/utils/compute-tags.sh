#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# Milestone 1: Compute docker image tags defaulting to v2
#
# By default, this script produces v2-style tags (no -v1 suffix).
# To produce v1-style tags, set VERSION=1 or JAEGER_VERSION=1 in the environment.
# To include both v2 and legacy v1 tags, set INCLUDE_LEGACY_V1=1 or pass --include-legacy-v1.
#
# Usage:
#   compute-tags.sh <base-image-name>
#   VERSION=1 compute-tags.sh <base-image-name>
#   INCLUDE_LEGACY_V1=1 compute-tags.sh <base-image-name>
#   compute-tags.sh --include-legacy-v1 <base-image-name>

# Compute major/minor/etc image tags based on the current branch

set -ef -o pipefail

if [[ -z $QUIET ]]; then
  set -x
fi

set -u

# Parse arguments for --include-legacy-v1 flag
INCLUDE_LEGACY_V1=${INCLUDE_LEGACY_V1:-0}
while [[ $# -gt 0 ]]; do
  case "$1" in
    --include-legacy-v1)
      INCLUDE_LEGACY_V1=1
      shift
      ;;
    *)
      break
      ;;
  esac
done

BASE_BUILD_IMAGE=${1:?'expecting Docker image name as argument, such as jaegertracing/jaeger'}
BRANCH=${BRANCH:?'expecting BRANCH env var'}
GITHUB_SHA=${GITHUB_SHA:-$(git rev-parse HEAD)}

# Default to v2 unless explicitly set to v1
VERSION=${VERSION:-${JAEGER_VERSION:-2}}

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
    
    if [[ "$VERSION" == "1" ]]; then
        # v1-only mode: produce v1 tags only
        tags "${BASE_BUILD_IMAGE}-v1:${MAJOR_MINOR_PATCH}"
        tags "${BASE_BUILD_IMAGE}-v1:latest"
    else
        # v2 mode (default): produce v2 tags first
        tags "${BASE_BUILD_IMAGE}:${MAJOR_MINOR_PATCH}"
        tags "${BASE_BUILD_IMAGE}:latest"
        
        # Optionally append v1 tags for compatibility
        if [[ "$INCLUDE_LEGACY_V1" == "1" ]]; then
            tags "${BASE_BUILD_IMAGE}-v1:${MAJOR_MINOR_PATCH}"
            tags "${BASE_BUILD_IMAGE}-v1:latest"
        fi
    fi
elif [[ $BRANCH != "main" ]]; then
    # not on release tag nor on main - no tags are needed since we won't publish
    echo ""
    exit
fi

# Snapshot tags (always produced regardless of version)
tags "${BASE_BUILD_IMAGE}-snapshot:${GITHUB_SHA}"
tags "${BASE_BUILD_IMAGE}-snapshot:latest"

echo "${IMAGE_TAGS}"
