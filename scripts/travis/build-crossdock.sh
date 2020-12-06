#!/bin/bash

set -euxf -o pipefail

BRANCH=${BRANCH:?'missing BRANCH env var'}
DOCKERHUB_LOGIN=${DOCKERHUB_LOGIN:-false}
COMMIT=${GITHUB_SHA::8}

make build-and-run-crossdock

# Only push the docker container to Docker Hub for master branch and when dockerhub login is done
if [[ "$BRANCH" == "master" && "$DOCKERHUB_LOGIN" == "true" ]]; then
  echo 'upload to Docker Hub'
else
  echo 'skip docker upload for PR'
  exit 0
fi

# docker image has been build when running the crossdock
export REPO=jaegertracing/test-driver
docker tag $REPO:latest $REPO:$COMMIT
bash ./scripts/travis/upload-to-docker.sh
