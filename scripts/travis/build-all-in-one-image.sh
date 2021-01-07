#!/bin/bash

set -exu

BRANCH=${BRANCH:?'missing BRANCH env var'}
# Set default GOARCH variable to the host GOARCH, the target architecture can
# be overrided by passing architecture value to the script:
# `GOARCH=<target arch> ./scripts/travis/build-all-in-one-image.sh`.
GOARCH=${GOARCH:-$(go env GOARCH)}
DOCKERHUB_LOGIN=${DOCKERHUB_LOGIN:-false}

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
  # Only push the docker container to Docker Hub for master/release branch and when dockerhub login is done
  if [[ ("$BRANCH" == "master" || $BRANCH =~ ^v[0-9]+\.[0-9]+\.[0-9]+$) && "$DOCKERHUB_LOGIN" == "true" ]]; then
    echo "upload $1 to Docker Hub"
    export REPO=$1
    bash ./scripts/travis/upload-to-docker.sh
  else
    echo 'skip docker upload for PR'
  fi
}

make build-all-in-one GOOS=linux GOARCH=$GOARCH
make create-baseimg-debugimg
repo=jaegertracing/all-in-one
docker build -f cmd/all-in-one/Dockerfile \
        --target release \
        --tag $repo:latest cmd/all-in-one \
        --build-arg base_image=localhost/baseimg:1.0.0-alpine-3.12 \
        --build-arg debug_image=localhost/debugimg:1.0.0-golang-1.15-alpine \
        --build-arg TARGETARCH=$GOARCH
run_integration_test $repo
upload_to_docker $repo

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

make build-otel-all-in-one GOOS=linux GOARCH=$GOARCH
repo=jaegertracing/opentelemetry-all-in-one
docker build -f cmd/opentelemetry/cmd/all-in-one/Dockerfile -t $repo:latest cmd/opentelemetry/cmd/all-in-one --build-arg TARGETARCH=$GOARCH
run_integration_test $repo
upload_to_docker $repo
