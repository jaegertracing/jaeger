#!/bin/bash

set -euxf -o pipefail

BRANCH=${BRANCH:?'missing BRANCH env var'}
COMMIT=${GITHUB_SHA::8}

make build-and-run-crossdock

# Only push images to dockerhub/quay.io for master branch
if [[ "$BRANCH" == "master" ]]; then
  echo 'upload images to dockerhub/quay.io'
  REPO=jaegertracing/test-driver
  IMAGE_TAGS=$(bash scripts/compute-tags.sh $REPO)
  IMAGE_TAGS="${IMAGE_TAGS} --tag docker.io/${REPO}:${COMMIT} --tag quay.io/${REPO}:${COMMIT}"
  bash scripts/docker-login.sh
  docker buildx build --push \
    --progress=plain \
    --platform=linux/amd64 \
    ${IMAGE_TAGS} \
    crossdock/
else
  echo 'skip docker images upload for PR'
fi
