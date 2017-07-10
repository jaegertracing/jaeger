#!/bin/bash

set -e

BRANCH=${BRANCH:?'missing BRANCH env var'}

source ~/.nvm/nvm.sh
nvm use 6
make build-all-in-one-linux

export REPO=jaegertracing/all-in-one

docker build -f cmd/standalone/Dockerfile -t $REPO:$COMMIT .
export CID=$(docker run -d -p 16686:16686 $REPO:$COMMIT)
make integration-test
docker kill $CID

# Only push the docker container to Docker Hub for master branch
if [[ "$BRANCH" == "master" && "$TRAVIS_SECURE_ENV_VARS" == "true" ]]; then
  echo 'upload to Docker Hub'
else
  echo 'skip docker upload for PR'
  exit 0
fi

bash ./travis/upload-to-docker.sh
