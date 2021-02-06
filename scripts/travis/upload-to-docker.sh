#!/bin/bash

set -euxf -o pipefail

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
  TAG="${BRANCH}"
fi
echo "REPO=$REPO, BRANCH=$BRANCH, TAG=$TAG, IMAGE=$IMAGE"

# add major, major.minor and major.minor.patch tags
if [[ -n ${major:-} ]]; then
  docker tag $IMAGE $REPO:${major}
  if [[ -n ${minor:-} ]]; then
    docker tag $IMAGE $REPO:${major}.${minor}
    if [[ -n ${patch:-} ]]; then
        docker tag $IMAGE $REPO:${major}.${minor}.${patch}
    fi
  fi
fi

wc -l DOCKERHUB_TOKEN.txt
cat DOCKERHUB_TOKEN.txt | docker login docker.io --username $DOCKERHUB_USER --password-stdin

echo login successful
exit 0

if [[ "${REPO}" == "jaegertracing/jaeger-opentelemetry-collector" || "${REPO}" == "jaegertracing/jaeger-opentelemetry-agent" || "${REPO}" == "jaegertracing/jaeger-opentelemetry-ingester" || "${REPO}" == "jaegertracing/opentelemetry-all-in-one" ]]; then
  # TODO remove once Jaeger OTEL collector is stable
  docker push $REPO:latest
else
  # push all tags, therefore push to repo
  docker push $REPO
fi

SNAPSHOT_IMAGE="$REPO-snapshot:$GITHUB_SHA"
echo "Pushing snapshot image $SNAPSHOT_IMAGE"
docker tag $IMAGE $SNAPSHOT_IMAGE
docker push $SNAPSHOT_IMAGE
