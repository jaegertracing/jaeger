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

# Do not enable echo before the `docker login` command to avoid revealing the password.
set -x
docker login -u $DOCKER_USER -p $DOCKER_PASS
if [[ "${REPO}" == "jaegertracing/jaeger-opentelemetry-collector" || "${REPO}" == "jaegertracing/jaeger-opentelemetry-agent" || "${REPO}" == "jaegertracing/jaeger-opentelemetry-ingester" ]]; then
  # TODO remove once Jaeger OTEL collector is stable
  docker push $REPO:latest
else
  # push all tags, therefore push to repo
  docker push $REPO
fi

SNAPSHOT_IMAGE="$REPO-snapshot:$TRAVIS_COMMIT"
echo "Pushing snapshot image $SNAPSHOT_IMAGE"
docker tag $IMAGE $SNAPSHOT_IMAGE
docker push $SNAPSHOT_IMAGE
