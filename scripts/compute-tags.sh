#!/bin/bash

# Compute major/minor/etc image tags based on the current branch

set -exu

BASE_BUILD_IMAGE=$1
BRANCH=${BRANCH:?'expecting BRANCH env var'}

## if we are on a release tag, let's extract the version number
## the other possible value, currently, is 'master' (or another branch name)
if [[ $BRANCH =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    MAJOR_MINOR_PATCH=$(echo ${BRANCH} | grep -Po "([\d\.]+)")
    MAJOR_MINOR=$(echo ${MAJOR_MINOR_PATCH} | awk -F. '{print $1"."$2}')
    MAJOR=$(echo ${MAJOR_MINOR_PATCH} | awk -F. '{print $1}')  
else
    MAJOR_MINOR_PATCH="latest"
    MAJOR_MINOR=""
    MAJOR=""
fi

# for docker.io and quay.io
BUILD_IMAGE=${BUILD_IMAGE:-"${BASE_BUILD_IMAGE}:${MAJOR_MINOR_PATCH}"}
IMAGE_TAGS="--tag docker.io/${BASE_BUILD_IMAGE} --tag docker.io/${BUILD_IMAGE} --tag quay.io/${BASE_BUILD_IMAGE} --tag quay.io/${BUILD_IMAGE}"
SNAPSHOT_TAG="${BASE_BUILD_IMAGE}-snapshot:${GITHUB_SHA}"

if [ "${MAJOR_MINOR}x" != "x" ]; then
    MAJOR_MINOR_IMAGE="${BASE_BUILD_IMAGE}:${MAJOR_MINOR}"
    IMAGE_TAGS="${IMAGE_TAGS} --tag docker.io/${MAJOR_MINOR_IMAGE} --tag quay.io/${MAJOR_MINOR_IMAGE}"
fi

if [ "${MAJOR}x" != "x" ]; then
    MAJOR_IMAGE="${BASE_BUILD_IMAGE}:${MAJOR}"
    IMAGE_TAGS="${IMAGE_TAGS} --tag docker.io/${MAJOR_IMAGE} --tag quay.io/${MAJOR_IMAGE}"
fi

IMAGE_TAGS="${IMAGE_TAGS} --tag docker.io/${SNAPSHOT_TAG} --tag quay.io/${SNAPSHOT_TAG}"

echo ${IMAGE_TAGS}
