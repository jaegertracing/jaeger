#!/bin/bash

set -euxf -o pipefail

BRANCH=${BRANCH:?'missing BRANCH env var'}
COMMIT=${GITHUB_SHA::8}

mode=${1-main}

make build-and-run-crossdock

if [[ "$mode" == "pr-only" ]]; then
  make create-baseimg
  # build artifacts for linux/amd64 only for pull requests
  platforms="linux/amd64"
else
  make create-baseimg-debugimg
  platforms="linux/amd64,linux/s390x,linux/ppc64le,linux/arm64"
  # build multi-arch binaries
  make build-binaries-s390x
  make build-binaries-ppc64le
  make build-binaries-arm64
fi

# Only push images to dockerhub/quay.io for the main branch
if [[ "$BRANCH" == "main" ]]; then
  echo 'upload images to dockerhub/quay.io'
  REPO=jaegertracing/test-driver
  IFS=" " read -r -a IMAGE_TAGS <<< "$(bash scripts/compute-tags.sh ${REPO})"
  IMAGE_TAGS+=("--tag" "docker.io/${REPO}:${COMMIT}" "--tag" "quay.io/${REPO}:${COMMIT}")
  bash scripts/docker-login.sh
  docker buildx build --push \
    --progress=plain \
    --platform=linux/amd64 \
    "${IMAGE_TAGS[@]}" \
    crossdock/
else
  echo 'skip docker images upload for PR'
fi
