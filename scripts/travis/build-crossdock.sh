#!/bin/bash

set -e

BRANCH=${BRANCH:?'missing BRANCH env var'}

make build-crossdock

# Only push the docker container to Docker Hub for master branch
if [[ "$BRANCH" == "master" && "$TRAVIS_SECURE_ENV_VARS" == "true" ]]; then
  echo 'upload to Docker Hub'
else
  echo 'skip docker upload for PR'
  exit 0
fi

set -x

# docker image has been build when running the crossdock
export REPO=jaegertracing/test-driver
docker tag $REPO:latest $REPO:$COMMIT
bash ./scripts/travis/upload-to-docker.sh
