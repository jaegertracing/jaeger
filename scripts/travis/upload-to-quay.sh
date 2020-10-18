#!/bin/bash

set -e

BRANCH=${BRANCH:?'missing BRANCH env var'}
IMAGE="${REPO:?'missing REPO env var'}:latest"
QUAY_URL="https://index.quay.io/v1"


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
docker login "$QUAY_URL" -u $QUAY_USER --password-stdin $QUAY_PASS 
if [[ "${REPO}" == "jaegertracing/jaeger-opentelemetry-collector" || "${REPO}" == "jaegertracing/jaeger-opentelemetry-agent" || "${REPO}" == "jaegertracing/jaeger-opentelemetry-ingester" || "${REPO}" == "jaegertracing/opentelemetry-all-in-one" ]]; then
  # TODO remove once Jaeger OTEL collector is stable
generate_image $IMAGE $QUAY_USER

else
  # push all tags, therefore push to repo
generate_image $IMAGE $QUAY_USER  
fi

generate_image (){
  ID = $(docker run -d $REPO)
  docker commit $ID quay.io/$2/$1
  docker push quay.io/$2/$1
  }

SNAPSHOT_IMAGE="$REPO-snapshot:$TRAVIS_COMMIT"
echo "Pushing snapshot image $SNAPSHOT_IMAGE"
docker tag $IMAGE $SNAPSHOT_IMAGE
docker push $SNAPSHOT_IMAGE
