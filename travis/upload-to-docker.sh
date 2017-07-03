#!/bin/bash

set -e

BRANCH=${BRANCH:?'missing BRANCH env var'}
IMAGE="${REPO:?'missing REPO env var'}:${COMMIT:?'missing COMMIT env var'}"

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

DOCKER=docker
$DOCKER login -u $DOCKER_USER -p $DOCKER_PASS

# Do not enable echo before the `docker login` command to avoid revealing the password.
set -x

$DOCKER tag $IMAGE $REPO:$TAG
$DOCKER tag $IMAGE $REPO:travis-$TRAVIS_BUILD_NUMBER
# add major and major.minor as aliases
if [[ -n $major ]]; then
  $DOCKER tag $IMAGE $REPO:$major
  if [[ -n $minor ]]; then
    $DOCKER tag $IMAGE $REPO:${major}.${minor}
  fi
fi

# TOOO why are we pushing as $REPO instead of $IMAGE?
$DOCKER push $REPO
