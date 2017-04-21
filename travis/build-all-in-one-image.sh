#!/bin/bash

set -e

source ~/.nvm/nvm.sh
nvm use 6
make build-all-in-one-linux

export REPO=jaegertracing/all-in-one
export BRANCH=$(if [ "$TRAVIS_PULL_REQUEST" == "false" ]; then echo $TRAVIS_BRANCH; else echo $TRAVIS_PULL_REQUEST_BRANCH; fi)

docker build -f cmd/standalone/Dockerfile -t $REPO:$COMMIT .
export CID=$(docker run -d -p 16686:16686 --rm $REPO:$COMMIT)
make integration-test
docker kill $CID

# Only push the docker container to Docker Hub for master branch
if [ "$BRANCH" == "master" ]; then echo 'upload to Docker Hub'; else echo 'skip docker upload for PR'; exit 0; fi

bash ./travis/upload-to-docker.sh
