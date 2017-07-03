#!/bin/bash

set -e

if [[ "$TRAVIS_SECURE_ENV_VARS" == "false" ]]; then
  echo "skip docker upload, TRAVIS_SECURE_ENV_VARS=$TRAVIS_SECURE_ENV_VARS"
  exit 0
fi

if [[ "$TRAVIS_PULL_REQUEST" == "false" ]]; then
  export BRANCH=$TRAVIS_BRANCH
else
  export BRANCH=$TRAVIS_PULL_REQUEST_BRANCH
fi

# Only push images to Docker Hub for master branch or for release tags vM.N.P
if [[ "$BRANCH" == "master" || $BRANCH =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "upload to Docker Hub, BRANCH=$BRANCH"
else
  echo 'skip docker upload for PR'
  exit 0
fi

source ~/.nvm/nvm.sh
nvm use 6

export DOCKER_NAMESPACE=jaegertracing
export DOCKER_TAG=${COMMIT:?'missing COMMIT env var'}
make docker

for component in agent cassandra-schema collector query
do
  export REPO="jaegertracing/jaeger-${component}"
  bash ./travis/upload-to-docker.sh
done
