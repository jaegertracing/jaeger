#!/bin/bash

set -e

export BRANCH=$(if [ "$TRAVIS_PULL_REQUEST" == "false" ]; then echo $TRAVIS_BRANCH; else echo $TRAVIS_PULL_REQUEST_BRANCH; fi)
# Only push the docker container to Docker Hub for master branch
if [[ "$BRANCH" == "master" && "$TRAVIS_SECURE_ENV_VARS" == "true" ]]; then echo 'upload to Docker Hub'; else echo 'skip docker upload for PR'; exit 0; fi

source ~/.nvm/nvm.sh
nvm use 6
DOCKER_NAMESPACE=jaegertracing make docker

for component in agent cassandra-schema collector query
do
  export REPO="jaegertracing/jaeger-${component}"
  bash ./travis/upload-to-docker.sh
done
