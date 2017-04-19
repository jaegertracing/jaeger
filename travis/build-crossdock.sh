#!/bin/bash

set -e

make build-crossdock

export REPO=jaegertracing/test-driver
export BRANCH=$(if [ "$TRAVIS_PULL_REQUEST" == "false" ]; then echo $TRAVIS_BRANCH; else echo $TRAVIS_PULL_REQUEST_BRANCH; fi)

# Only push the docker container to Docker Hub for master branch
#if [ "$BRANCH" == "master" ]; then echo 'upload to Docker Hub'; else echo 'skip docker upload for PR'; exit 0; fi

docker build -f crossdock/Dockerfile -t $REPO:$COMMIT .

./travis/upload-to-docker.sh
