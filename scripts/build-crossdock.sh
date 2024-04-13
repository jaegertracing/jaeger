#!/bin/bash

set -euxf -o pipefail

BRANCH=${BRANCH:?'missing BRANCH env var'}
COMMIT=${GITHUB_SHA::8}

make build-and-run-crossdock

# Only push images to dockerhub/quay.io for the main branch
if [[ "$BRANCH" == "main" ]]; then
  echo 'upload images to dockerhub/quay.io'
  REPO=jaegertracing/test-driver

  TARGET_ARCH="linux/arm64"

  IMAGE_TAGS=("--tag" "docker.io/${REPO}:${COMMIT}-${TARGET_ARCH}" "--tag" "quay.io/${REPO}:${COMMIT}-${TARGET_ARCH}")

  # Build the image for the target architecture
  bash scripts/docker-login.sh
  docker buildx build --push \
    --progress=plain \
    --platform "${TARGET_ARCH}" \
    "${IMAGE_TAGS[@]}" \
    crossdock/

else
  echo 'skip docker images upload for PR'
fi
