#!/bin/bash

set -e

if [[ -z "$BRANCH" ]]; then
  BRANCH=$(git rev-parse --abbrev-ref HEAD)
  echo "BRANCH env var not defined, using current branch $BRANCH instead ..."
fi

if [[ (-z "$SKIP_UI") || ($SKIP_UI == false) ]]; then
  # Only build the UI on master branch.
  source ~/.nvm/nvm.sh
  nvm use 10
  make build-ui
  make build-all-in-one GOOS=linux
else
  make build-all-in-one-without-ui GOOS=linux
fi

set -x

export REPO=jaegertracing/all-in-one
docker build -f cmd/all-in-one/Dockerfile -t $REPO:latest cmd/all-in-one
export CID=$(docker run -d -p 16686:16686 -p 5778:5778 $REPO:latest)

set +e
make all-in-one-integration-test

if [ $? -ne 0 ] ; then
    echo "---- integration test failed unexpectedly ----"
    echo "--- check the docker log below for details ---"
    docker logs $CID
    exit 1
fi
set -e

docker kill $CID

# Only push the docker container to Docker Hub for master branch
if [[ ("$BRANCH" == "master" || $BRANCH =~ ^v[0-9]+\.[0-9]+\.[0-9]+$) && "$TRAVIS_SECURE_ENV_VARS" == "true" ]]; then
  echo 'upload to Docker Hub'
else
  echo 'skip docker upload for PR'
  exit 0
fi

bash ./scripts/travis/upload-to-docker.sh
