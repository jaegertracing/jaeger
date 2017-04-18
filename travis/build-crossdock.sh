#!/bin/bash

set -e

make build-crossdock

export REPO=jaegertracing/test-driver
export BRANCH=$(if [ "$TRAVIS_PULL_REQUEST" == "false" ]; then echo $TRAVIS_BRANCH; else echo $TRAVIS_PULL_REQUEST_BRANCH; fi)
export TAG=`if [ "$BRANCH" == "master" ]; then echo "latest"; else echo "${BRANCH///}"; fi`
echo "TRAVIS_BRANCH=$TRAVIS_BRANCH, REPO=$REPO, PR=$PR, BRANCH=$BRANCH, TAG=$TAG"

# Only push the docker container to Docker Hub for master branch
#if [ "$BRANCH" == "master" ]; then echo 'upload to Docker Hub'; else echo 'skip docker upload for PR'; exit 0; fi

docker login -u $DOCKER_USER -p $DOCKER_PASS

set -x

docker build -f crossdock/Dockerfile -t $REPO:$COMMIT .
docker tag $REPO:$COMMIT $REPO:$TAG
docker tag $REPO:$COMMIT $REPO:travis-$TRAVIS_BUILD_NUMBER
docker push $REPO
