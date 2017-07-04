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

export REPO=jaegertracing/test-driver
docker build -t $REPO:$COMMIT ./crossdock

bash ./travis/upload-to-docker.sh
