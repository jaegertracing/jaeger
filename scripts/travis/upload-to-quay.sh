#!/bin/bash

set -e

BRANCH=${BRANCH:?'missing BRANCH env var'}
IMAGE="${REPO:?'missing REPO env var'}:latest"

unset major minor patch
if [[ "$BRANCH" == "master" ]]; then
  TAG="latest"
elif [[ $BRANCH =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
  major="${BASH_REMATCH[1]}"
  minor="${BASH_REMATCH[2]}"
  patch="${BASH_REMATCH[3]}"
  TAG=${major}.${minor}.${patch}
  echo "BRANCH is a release tag: major=$major, minor=$minor, patch=$patch"
else
  # TODO why do we do /// ?
  TAG="${BRANCH///}"
fi
echo "TRAVIS_BRANCH=$TRAVIS_BRANCH, REPO=$REPO, BRANCH=$BRANCH, TAG=$TAG, IMAGE=$IMAGE"

# add major, major.minor and major.minor.patch tags
if [[ -n $major ]]; then
  docker tag $IMAGE $REPO:${major}
  if [[ -n $minor ]]; then
    docker tag $IMAGE $REPO:${major}.${minor}
    if [[ -n $patch ]]; then
        docker tag $IMAGE $REPO:${major}.${minor}.${patch}
    fi
  fi
fi

if [[ -f $HOME/.docker/config.json ]]; then
  rm -f $HOME/.docker/config.json
else
  echo "$HOME/.docker/config.json doesn't exist"
fi

# Do not enable echo before the `docker login` command to avoid revealing the password.
set -x
docker login quay.io -u $QUAY_USER -p $QUAY_PASS 
echo "Quay login successful"
QUAY_IMAGE="$REPO:$TAG"
echo $QUAY_IMAGE

function strip_image_name {
  SUBSTRING=$(echo $1| cut -d'/' -f 2)
  QUAY_IMAGE_NAME=$(echo $SUBSTRING| cut -d':' -f 1)
  QUAY_IMAGE_TAG=$(echo $SUBSTRING| cut -d':' -f 2)
  echo "$QUAY_IMAGE_NAME:$QUAY_IMAGE_TAG"
}

function push_to_quay {
  docker pull $1
  IMAGE_ID=$(docker images $1 -q)
  echo "the image id is:" $IMAGE_ID
  docker tag $IMAGE_ID "quay.io/aebirim/$2:$3"
  docker push "quay.io/aebirim/$2:$3"
  #delete the pulled image
  docker rmi -f $IMAGE_ID
}

if [[ "${REPO}" == "jaegertracing/jaeger-opentelemetry-collector" || "${REPO}" == "jaegertracing/jaeger-opentelemetry-agent" || "${REPO}" == "jaegertracing/jaeger-opentelemetry-ingester" || "${REPO}" == "jaegertracing/opentelemetry-all-in-one" ]]; then
  # TODO remove once Jaeger OTEL collector is stable
QUAY_IMAGE_NAME_TAG=$(strip_image_name $QUAY_IMAGE)  
QUAY_IMAGE_NAME=$(echo $QUAY_IMAGE_NAME_TAG| cut -d':' -f 1)
push_to_quay $QUAY_IMAGE $QUAY_IMAGE_NAME "latest" 

elif [[ "${REPO}" == "$REPO-snapshot"]]; then
QUAY_SNAPSHOT_IMAGE="$REPO-snapshot:$TRAVIS_COMMIT"
echo "pushing snapshot image to quay.io:" QUAY_SNAPSHOT_IMAGE
QUAY_SNAPSHOT_IMAGE_NAME_TAG=$(strip_image_name $QUAY_SNAPSHOT_IMAGE)
QUAY_SNAPSHOT_IMAGE_NAME=$(echo $QUAY_SNAPSHOT_IMAGE_NAME_TAG| cut -d':' -f 1)
QUAY_SNAPSHOT_IMAGE_TAG=$(echo $QUAY_SNAPSHOT_IMAGE_NAME_TAG| cut -d':' -f 2)
push_to_quay $QUAY_SNAPSHOT_IMAGE $QUAY_SNAPSHOT_IMAGE_NAME $QUAY_SNAPSHOT_IMAGE_TAG

else
# push all tags, therefore push to repo
QUAY_IMAGE_NAME_TAG=$(strip_image_name $QUAY_IMAGE)
QUAY_IMAGE_NAME=$(echo $QUAY_IMAGE_NAME_TAG| cut -d':' -f 1)
QUAY_IMAGE_TAG=$(echo $QUAY_IMAGE_NAME_TAG| cut -d':' -f 2)
push_to_quay $QUAY_IMAGE $QUAY_IMAGE_NAME $QUAY_IMAGE_TAG
fi

#QUAY_SNAPSHOT_IMAGE="$REPO-snapshot:$TRAVIS_COMMIT"
#echo "Pushing snapshot image to quay.io:" $QUAY_SNAPSHOT_IMAGE
#QUAY_SNAPSHOT_IMAGE_NAME_TAG=$(strip_image_name $QUAY_SNAPSHOT_IMAGE)
#QUAY_SNAPSHOT_IMAGE_NAME=$(echo $QUAY_SNAPSHOT_IMAGE_NAME_TAG| cut -d':' -f 1)
#QUAY_SNAPSHOT_IMAGE_TAG=$(echo $QUAY_SNAPSHOT_IMAGE_NAME_TAG| cut -d':' -f 2)
#push_to_quay $QUAY_SNAPSHOT_IMAGE $QUAY_SNAPSHOT_IMAGE_NAME $QUAY_SNAPSHOT_IMAGE_TAG
