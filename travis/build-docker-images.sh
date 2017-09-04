#!/bin/bash

set -e

if [[ "$TRAVIS_SECURE_ENV_VARS" == "false" ]]; then
  echo "skip docker upload, TRAVIS_SECURE_ENV_VARS=$TRAVIS_SECURE_ENV_VARS"
  exit 0
fi

BRANCH=${BRANCH:?'missing BRANCH env var'}

# Only push images to Docker Hub for master branch or for release tags vM.N.P
if [[ "$BRANCH" == "master" || $BRANCH =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "upload to Docker Hub, BRANCH=$BRANCH"
else
  echo 'skip Docker upload, only allowed for tagged releases or master (latest tag)'
  exit 0
fi

source ~/.nvm/nvm.sh
nvm use 6

export DOCKER_NAMESPACE=jaegertracing
make docker

for component in agent cassandra-schema collector query
do
  export REPO="jaegertracing/jaeger-${component}"
  bash ./travis/upload-to-docker.sh
done
