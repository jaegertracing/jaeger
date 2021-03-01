#!/bin/bash

set -euxf -o pipefail

BRANCH=${BRANCH:?'missing BRANCH env var'}
COMMIT=${GITHUB_SHA::8}

make build-and-run-crossdock

# Only push images to dockerhub/quay.io for master branch
if [[ "$BRANCH" == "master" ]]; then
  echo 'upload images to dockerhub/quay.io'
else
  echo 'skip docker images upload for PR'
  exit 0
fi

# docker image has been build when running the crossdock
REPO=jaegertracing/test-driver
docker tag $REPO:latest $REPO:$COMMIT
bash scripts/upload-to-registry.sh $REPO
