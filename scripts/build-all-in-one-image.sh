#!/bin/bash

set -exu

BRANCH=${BRANCH:?'missing BRANCH env var'}
# Set default GOARCH variable to the host GOARCH, the target architecture can
# be overrided by passing architecture value to the script:
# `GOARCH=<target arch> ./scripts/build-all-in-one-image.sh`.
GOARCH=${GOARCH:-$(go env GOARCH)}

expected_version="v10"
version=$(node --version)
major_version=${version%.*.*}
if [ "$major_version" = "$expected_version" ] ; then
  echo "Node version is as expected: $version"
else
  echo "ERROR: installed Node version $version doesn't match expected version $expected_version"
  exit 1
fi

make build-ui

set +e

run_integration_test() {
  CID=$(docker run -d -p 16686:16686 -p 5778:5778 $1:latest)
  make all-in-one-integration-test
  if [ $? -ne 0 ] ; then
      echo "---- integration test failed unexpectedly ----"
      echo "--- check the docker log below for details ---"
      docker logs $CID
      exit 1
  fi
  docker kill $CID
}

upload_to_docker() {
  # Only push the docker image to dockerhub/quay.io for master/release branch
  if [[ "$BRANCH" == "master" || $BRANCH =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "upload $1 to dockerhub/quay.io"
    REPO=$1
    bash scripts/upload-to-registry.sh $REPO
  else
    echo 'skip docker images upload for PR'
  fi
}

upload_multiarch_to_docker() {
  # Only push the docker image to dockerhub/quay.io for master/release branch
  if [[ "$BRANCH" == "master" || $BRANCH =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "upload $1 to dockerhub/quay.io"
    REPO=$1
    build_upload_multiarch_to_docker $REPO
  else
    echo 'skip docker images upload for PR'
  fi
}

build_upload_multiarch_to_docker(){
  DOCKERHUB_USERNAME=${DOCKERHUB_USERNAME:-"jaegertracingbot"}
  DOCKERHUB_TOKEN=${DOCKERHUB_TOKEN:-}
  QUAY_USERNAME=${QUAY_USERNAME:-"jaegertracing+github_workflows"}
  QUAY_TOKEN=${QUAY_TOKEN:-}

  ###############Compute the tag
  BASE_BUILD_IMAGE=$1

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
  ################################

  # Only push images to dockerhub/quay.io for master branch or for release tags vM.N.P
  echo "build multiarch images and upload to dockerhub/quay.io, BRANCH=$BRANCH"

  echo "Performing a 'docker login' for DockerHub"
  echo "${DOCKERHUB_TOKEN}" | docker login -u "${DOCKERHUB_USERNAME}" docker.io --password-stdin

  echo "Performing a 'docker login' for Quay"
  echo "${QUAY_TOKEN}" | docker login -u "${QUAY_USERNAME}" quay.io --password-stdin

  docker buildx build --output "type=image, push=true" \
    --progress=plain --target release \
    --build-arg base_image="localhost:5000/baseimg:1.0.0-alpine-3.12" \
    --build-arg debug_image="golang:1.15-alpine" \
    --platform=$PLATFORMS \
    --file cmd/all-in-one/Dockerfile \
    ${IMAGE_TAGS} \
    cmd/all-in-one
}

make build-all-in-one GOOS=linux GOARCH=amd64
make build-all-in-one GOOS=linux GOARCH=s390x

make create-baseimage-multiarch
make create-baseimg-debugimg
repo=jaegertracing/all-in-one

PLATFORMS="linux/amd64,linux/s390x"

docker buildx build --push \
    --progress=plain --target release \
    --build-arg base_image="localhost:5000/baseimg:1.0.0-alpine-3.12" \
    --build-arg debug_image="golang:1.15-alpine" \
    --platform=$PLATFORMS \
    --file cmd/all-in-one/Dockerfile \
    --tag localhost:5000/$repo:latest \
    cmd/all-in-one
run_integration_test localhost:5000/$repo
upload_multiarch_to_docker $repo

make build-all-in-one-debug GOOS=linux GOARCH=$GOARCH
repo=jaegertracing/all-in-one-debug
docker build -f cmd/all-in-one/Dockerfile \
        --target debug \
        --tag $repo:latest cmd/all-in-one \
        --build-arg base_image=localhost/baseimg:1.0.0-alpine-3.12 \
        --build-arg debug_image=localhost/debugimg:1.0.0-golang-1.15-alpine \
        --build-arg TARGETARCH=$GOARCH
run_integration_test $repo
upload_to_docker $repo
