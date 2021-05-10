#!/bin/bash

set -euxf -o pipefail

BRANCH=${BRANCH:?'missing BRANCH env var'}
COMMIT=${GITHUB_SHA::8}

make build-and-run-crossdock

# Only push images to dockerhub/quay.io for master branch
if [[ "$BRANCH" == "master" ]]; then
  echo 'upload images to dockerhub/quay.io'
  make build-crossdock-binary GOOS=linux GOARCH=amd64
  make build-crossdock-binary GOOS=linux GOARCH=s390x
  PLATFORMS="linux/amd64,linux/s390x"
  REPO=jaegertracing/test-driver
  IMAGE_TAGS=$(bash scripts/compute-tags.sh $REPO)
  IMAGE_TAGS="${IMAGE_TAGS} --tag docker.io/${REPO}:${COMMIT} --tag quay.io/${REPO}:${COMMIT}"
  bash scripts/docker-login.sh
  docker buildx build --push \
    --progress=plain \
    --platform=$PLATFORMS \
    ${IMAGE_TAGS} \
    crossdock/
else
  echo 'skip docker images upload for PR'
fi
