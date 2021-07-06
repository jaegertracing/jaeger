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

make create-baseimg-debugimg

make build-all-in-one GOOS=linux GOARCH=amd64
make build-all-in-one GOOS=linux GOARCH=s390x
make build-all-in-one GOOS=linux GOARCH=arm64
platforms="linux/amd64,linux/s390x,linux/arm64"
repo=jaegertracing/all-in-one
#build all-in-one image locally for integration test
bash scripts/build-upload-a-docker-image.sh -l -b -c all-in-one -d cmd/all-in-one -p "${platforms}" -t release
run_integration_test localhost:5000/$repo
#build all-in-one image and upload to dockerhub/quay.io
bash scripts/build-upload-a-docker-image.sh -b -c all-in-one -d cmd/all-in-one -p "${platforms}" -t release


make build-all-in-one-debug GOOS=linux GOARCH=$GOARCH
repo=${repo}-debug
#build all-in-one-debug image locally for integration test
bash scripts/build-upload-a-docker-image.sh -l -b -c all-in-one-debug -d cmd/all-in-one -t debug
run_integration_test localhost:5000/$repo
#build all-in-one-debug image and upload to dockerhub/quay.io
bash scripts/build-upload-a-docker-image.sh -b -c all-in-one-debug -d cmd/all-in-one -t debug
