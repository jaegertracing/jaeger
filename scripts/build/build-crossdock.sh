#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euxf -o pipefail

BRANCH=${BRANCH:?'missing BRANCH env var'}
COMMIT=${GITHUB_SHA::8}

make build-and-run-crossdock

# Only push images to dockerhub/quay.io for the main branch
if [[ "$BRANCH" == "main" ]]; then
  echo 'upload images to dockerhub/quay.io'
  REPO=jaegertracing/test-driver
  IFS=" " read -r -a IMAGE_TAGS <<< "$(bash scripts/utils/compute-tags.sh ${REPO})"
  IMAGE_TAGS+=("--tag" "docker.io/${REPO}:${COMMIT}" "--tag" "quay.io/${REPO}:${COMMIT}")
  bash scripts/utils/docker-login.sh

  docker buildx build --push \
    --progress=plain \
    --platform=linux/amd64 \
    "${IMAGE_TAGS[@]}" \
    crossdock/
else
  echo 'skip docker images upload for PR'
fi
