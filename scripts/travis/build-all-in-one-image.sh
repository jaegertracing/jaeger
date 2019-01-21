#!/bin/bash

set -e

BRANCH=${BRANCH:?'missing BRANCH env var'}

source ~/.nvm/nvm.sh
nvm use 8
make build-all-in-one-linux

export REPO=jaegertracing/all-in-one

docker build -f cmd/all-in-one/Dockerfile -t $REPO:latest .
export CID=$(docker run -d -p 16686:16686 -p 5778:5778 $REPO:latest)
make integration-test
docker kill $CID

# Only push the docker container to Docker Hub for master branch
if [[ ("$BRANCH" == "master" || $BRANCH =~ ^v[0-9]+\.[0-9]+\.[0-9]+$) && "$TRAVIS_SECURE_ENV_VARS" == "true" ]]; then
  echo 'upload to Docker Hub'
else
  echo 'skip docker upload for PR'
  exit 0
fi

bash ./scripts/travis/upload-to-docker.sh
