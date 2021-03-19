#!/bin/bash

set -euxf -o pipefail

DOCKERHUB_USERNAME=${DOCKERHUB_USERNAME:-"jaegertracingbot"}
DOCKERHUB_TOKEN=${DOCKERHUB_TOKEN:-}
QUAY_USERNAME=${QUAY_USERNAME:-"jaegertracing+github_workflows"}
QUAY_TOKEN=${QUAY_TOKEN:-}

###############Compute the tag
BASE_BUILD_IMAGE=${BASE_BUILD_IMAGE:-"jaegertracing/jaeger-JAGERCOMP"}

## if we are on a release tag, let's extract the version number
## the other possible value, currently, is 'master' (or another branch name)
if [[ $BRANCH == v* ]]; then
    COMPONENT_VERSION=$(echo ${BRANCH} | grep -Po "([\d\.]+)")
    MAJOR_MINOR=$(echo ${COMPONENT_VERSION} | awk -F. '{print $1"."$2}')
    MAJOR1=$(echo ${COMPONENT_VERSION} | awk -F. '{print $1}')
  
else
    COMPONENT_VERSION="latest"
    MAJOR_MINOR=""
    MAJOR1=""
fi

# for docker.io
BUILD_IMAGE=${BUILD_IMAGE:-"${BASE_BUILD_IMAGE}:${COMPONENT_VERSION}"}
IMAGE_TAGS="--tag docker.io/${BASE_BUILD_IMAGE} --tag docker.io/${BUILD_IMAGE}"
SNAPSHOT_TAG="${BASE_BUILD_IMAGE}-snapshot:${GITHUB_SHA}"

if [ "${MAJOR_MINOR}x" != "x" ]; then
    MAJOR_MINOR_IMAGE="${BASE_BUILD_IMAGE}:${MAJOR_MINOR}"
    IMAGE_TAGS="${IMAGE_TAGS} --tag docker.io/${MAJOR_MINOR_IMAGE}"
fi

if [ "${MAJOR1}x" != "x" ]; then
    MAJOR1_IMAGE="${BASE_BUILD_IMAGE}:${MAJOR1}"
    IMAGE_TAGS="${IMAGE_TAGS} --tag docker.io/${MAJOR1_IMAGE}"
fi

IMAGE_TAGS="${IMAGE_TAGS} --tag docker.io/${SNAPSHOT_TAG}"

## for quay.io
IMAGE_TAGS="${IMAGE_TAGS} --tag quay.io/${BASE_BUILD_IMAGE} --tag quay.io/${BUILD_IMAGE}"

if [ "${MAJOR_MINOR}x" != "x" ]; then
    IMAGE_TAGS="${IMAGE_TAGS} --tag quay.io/${MAJOR_MINOR_IMAGE}"
fi

if [ "${MAJOR1}x" != "x" ]; then
    IMAGE_TAGS="${IMAGE_TAGS} --tag quay.io/${MAJOR1_IMAGE}"
fi

IMAGE_TAGS="${IMAGE_TAGS} --tag quay.io/${SNAPSHOT_TAG}"
#################################

# Only push multi-arch images to dockerhub/quay.io for master branch or for release tags vM.N.P
if [[ "$BRANCH" == "master" || $BRANCH =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "build multiarch images and upload to dockerhub/quay.io, BRANCH=$BRANCH"

  echo "Performing a 'docker login' for DockerHub"
  echo "${DOCKERHUB_TOKEN}" | docker login -u "${DOCKERHUB_USERNAME}" docker.io --password-stdin

  echo "Performing a 'docker login' for Quay"
  echo "${QUAY_TOKEN}" | docker login -u "${QUAY_USERNAME}" quay.io --password-stdin

  IMAGE_TAGS="${IMAGE_TAGS}" PUSHTAG="type=image, push=true" make multiarch-docker
else
  echo 'skip multiarch docker images upload, only allowed for tagged releases or master (latest tag)'
  IMAGE_TAGS="${IMAGE_TAGS}" PUSHTAG="type=image, push=false" make multiarch-docker
fi


make docker-images-jaeger-backend-debug
make docker-images-cassandra
# Only push amd64 specific images to dockerhub/quay.io for master branch or for release tags vM.N.P
if [[ "$BRANCH" == "master" || $BRANCH =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "upload to dockerhub/quay.io, BRANCH=$BRANCH"
else
  echo 'skip docker images upload, only allowed for tagged releases or master (latest tag)'
  exit 0
fi

export DOCKER_NAMESPACE=jaegertracing

jaeger_components=(
	agent-debug
	cassandra-schema
	collector-debug
	query-debug
	ingester-debug
)

for component in "${jaeger_components[@]}"
do
  REPO="jaegertracing/jaeger-${component}"
  bash scripts/upload-to-registry.sh $REPO
done

