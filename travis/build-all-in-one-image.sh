#!/bin/bash

set -ex

export REPO=jaegertracing/all-in-one
export BRANCH=$(if [ "$TRAVIS_PULL_REQUEST" == "false" ]; then echo $TRAVIS_BRANCH; else echo $TRAVIS_PULL_REQUEST_BRANCH; fi)
export TAG=`if [ "$BRANCH" == "master" ]; then echo "latest"; else echo "${BRANCH///}"; fi`
echo "TRAVIS_BRANCH=$TRAVIS_BRANCH, REPO=$REPO, PR=$PR, BRANCH=$BRANCH, TAG=$TAG"

make build-all-in-one-linux

# Only push the docker container to Docker Hub for master branch
#if [[ "$TRAVIS_PULL_REQUEST" == "false"  || "$BRANCH" == "master" ]]; then echo 'upload to Docker Hub'; else echo 'skip docker upload for PR'; exit 0; fi

docker login -u $DOCKER_USER -p $DOCKER_PASS
docker build -f cmd/standalone/Dockerfile -t $REPO:$COMMIT .
docker tag $REPO:$COMMIT $REPO:$TAG
docker tag $REPO:$COMMIT $REPO:travis-$TRAVIS_BUILD_NUMBER
docker push $REPO
