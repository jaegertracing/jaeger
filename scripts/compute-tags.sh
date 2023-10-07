#!/bin/bash

# Compute major/minor/etc image tags based on the current branch

set -eux

BASE_BUILD_IMAGE=$1
GITHUB_SHA=${GITHUB_SHA:-$(git rev-parse HEAD)}
VERSION=${VERSION:-$(make echo-version)}

if [[ $VERSION =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    VERSION="${VERSION#[vV]}"
    VERSION_MAJOR="${VERSION%%\.*}"
    VERSION_MINOR="${VERSION#*.}"
    VERSION_MINOR="${VERSION_MINOR%.*}"
    VERSION_PATCH="${VERSION##*.}"
else
    VERSION="latest"
fi

# for docker.io and quay.io
BUILD_IMAGE=${BUILD_IMAGE:-"${BASE_BUILD_IMAGE}:${VERSION}"}
IMAGE_TAGS="--tag docker.io/${BASE_BUILD_IMAGE} --tag docker.io/${BUILD_IMAGE} --tag quay.io/${BASE_BUILD_IMAGE} --tag quay.io/${BUILD_IMAGE}"
SNAPSHOT_TAG="${BASE_BUILD_IMAGE}-snapshot:${GITHUB_SHA}"


if [ "${VERSION}" != "latest" ]; then
    MAJOR_MINOR_IMAGE="${BASE_BUILD_IMAGE}:${VERSION_MAJOR}.${VERSION_MINOR}"
    IMAGE_TAGS="${IMAGE_TAGS} --tag docker.io/${MAJOR_MINOR_IMAGE} --tag quay.io/${MAJOR_MINOR_IMAGE}"

    # TODO we should stop publishing these, as everything ends up with tag "1" but not backwards compatible
    MAJOR_IMAGE="${BASE_BUILD_IMAGE}:${VERSION_MAJOR}"
    IMAGE_TAGS="${IMAGE_TAGS} --tag docker.io/${MAJOR_IMAGE} --tag quay.io/${MAJOR_IMAGE}"
fi

IMAGE_TAGS="${IMAGE_TAGS} --tag docker.io/${SNAPSHOT_TAG} --tag quay.io/${SNAPSHOT_TAG}"

echo ${IMAGE_TAGS}
