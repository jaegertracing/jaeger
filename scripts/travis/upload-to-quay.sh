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
echo "Quay login successful "

 #remove org name jaegertracing to get image name
function strip_image_name {
  SUBSTRING_1=$(echo $1| cut -d'/' -f 2)
  SUBSTRING_2=$(echo $SUBSTRING_1| cut -d':' -f 1)
  echo $SUBSTRING_2
}

function push_to_quay {
  ID=$(docker run -d $2)
 #strip container ID to 12 characters
  STRIPPED_ID=$(echo $ID| cut -b 1-12)
  docker commit $STRIPPED_ID quay.io/aebirim/$1
  docker push quay.io/aebirim/$1
}

if [[ "${REPO}" == "jaegertracing/jaeger-opentelemetry-collector" || "${REPO}" == "jaegertracing/jaeger-opentelemetry-agent" || "${REPO}" == "jaegertracing/jaeger-opentelemetry-ingester" || "${REPO}" == "jaegertracing/opentelemetry-all-in-one" ]]; then
  # TODO remove once Jaeger OTEL collector is stable

STRIPPED_IMAGE=$(strip_image_name $IMAGE)  
push_to_quay $STRIPPED_IMAGE $REPO

else
  # push all tags, therefore push to repo
STRIPPED_IMAGE=$(strip_image_name $IMAGE)  
push_to_quay $STRIPPED_IMAGE $REPO  
fi

SNAPSHOT_IMAGE="$REPO-snapshot:$TRAVIS_COMMIT"
echo "Pushing snapshot image $SNAPSHOT_IMAGE"
docker tag $IMAGE $SNAPSHOT_IMAGE
docker push $SNAPSHOT_IMAGE
