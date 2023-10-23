#!/bin/bash

set -exu

# Set default GOARCH variable to the host GOARCH, the target architecture can
# be overrided by passing architecture value to the script:
# `GOARCH=<target arch> ./scripts/build-all-in-one-image.sh`.
GOARCH=${GOARCH:-$(go env GOARCH)}
mode=${1-main}
repo=jaegertracing/all-in-one

# verify Node.js version
expected_version="v$(cat jaeger-ui/.nvmrc)"
version=$(node --version)
major_version=${version%.*.*}

if [ "$major_version" = "$expected_version" ] ; then
  echo "Node version is as expected: $version"
else
  echo "ERROR: installed Node version $version doesn't match expected version $expected_version"
  exit 1
fi

make build-ui

run_integration_test() {
  local image_name="$1"
  CID=$(docker run -d -p 16686:16686 -p 5778:5778 "${image_name}:${GITHUB_SHA}")
  if ! make all-in-one-integration-test ; then
      echo "---- integration test failed unexpectedly ----"
      echo "--- check the docker log below for details ---"
      docker logs "$CID"
      exit 1
  fi
  docker kill "$CID"
}

if [ "$mode" = "pr-only" ]; then
  make create-baseimg
  # build architecture for linux/amd64 only for pull requests
  platforms="linux/amd64"
  make build-all-in-one GOOS=linux GOARCH=amd64
else
  make create-baseimg-debugimg
  platforms="linux/amd64,linux/s390x,linux/ppc64le,linux/arm64"
  make build-all-in-one GOOS=linux GOARCH=amd64
  make build-all-in-one GOOS=linux GOARCH=s390x
  make build-all-in-one GOOS=linux GOARCH=ppc64le
  make build-all-in-one GOOS=linux GOARCH=arm64
fi

# build all-in-one image locally for integration test
bash scripts/build-upload-a-docker-image.sh -l -b -c all-in-one -d cmd/all-in-one -p "${platforms}" -t release
run_integration_test localhost:5000/$repo

# build all-in-one image and upload to dockerhub/quay.io
bash scripts/build-upload-a-docker-image.sh -b -c all-in-one -d cmd/all-in-one -p "${platforms}" -t release

# build debug image if not on a pull request
if [ "$mode" != "pr-only" ]; then
  make build-all-in-one GOOS=linux GOARCH="$GOARCH" DEBUG_BINARY=1
  repo=${repo}-debug

  # build all-in-one DEBUG image locally for integration test
  bash scripts/build-upload-a-docker-image.sh -l -b -c all-in-one-debug -d cmd/all-in-one -t debug
  run_integration_test localhost:5000/$repo

  # build all-in-one-debug image and upload to dockerhub/quay.io
  bash scripts/build-upload-a-docker-image.sh -b -c all-in-one-debug -d cmd/all-in-one -t debug
fi
