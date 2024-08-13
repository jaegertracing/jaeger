#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euf -o pipefail

print_help() {
  echo "Usage: $0 [-b binary] [-D] [-l] [-p platforms]"
  echo "-b: Which binary to build: 'all-in-one' (default) or 'jaeger' (v2)"
  echo "-D: Disable building of images with debugger"
  echo "-h: Print help"
  echo "-l: Enable local-only mode that only pushes images to local registry"
  echo "-p: Comma-separated list of platforms to build for (default: all supported)"
  exit 1
}

add_debugger='Y'
platforms="linux/amd64,linux/s390x,linux/ppc64le,linux/arm64"
LOCAL_FLAG=''
BINARY='all-in-one'

while getopts "b:Dhlp:" opt; do
	case "${opt}" in
	b)
		BINARY=${OPTARG}
		;;
	D)
		add_debugger='N'
		;;
	l)
		# in the local-only mode the images will only be pushed to local registry
		LOCAL_FLAG='-l'
		;;
	p)
		platforms=${OPTARG}
		;;
	?)
		print_help
		;;
	esac
done

set -x

# Set default GOARCH variable to the host GOARCH, the target architecture can
# be overrided by passing architecture value to the script:
# `GOARCH=<target arch> ./scripts/build-all-in-one-image.sh`.
GOARCH=${GOARCH:-$(go env GOARCH)}
repo="jaegertracing/${BINARY}"

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
      echo "::group::docker logs"
        docker logs "$CID"
      echo "::endgroup::"
      docker kill "$CID"
      exit 1
  fi
  docker kill "$CID"
}

# Loop through each platform (separated by commas)
for platform in $(echo "$platforms" | tr ',' ' '); do
  # Extract the architecture from the platform string
  arch=${platform##*/}  # Remove everything before the last slash
  make "build-${BINARY}" GOOS=linux GOARCH="${arch}"
done

if [[ "${add_debugger}" == "N" ]]; then
  make create-baseimg
else
  make create-baseimg-debugimg
fi

# build all-in-one image locally for integration test (the -l switch)
bash scripts/build-upload-a-docker-image.sh -l -b -c "${BINARY}" -d "cmd/${BINARY}" -p "${platforms}" -t release

run_integration_test "localhost:5000/$repo"

# build all-in-one image and upload to dockerhub/quay.io
bash scripts/build-upload-a-docker-image.sh ${LOCAL_FLAG} -b -c "${BINARY}" -d "cmd/${BINARY}" -p "${platforms}" -t release

# build debug image if requested
if [[ "${add_debugger}" == "Y" ]]; then
  make "build-${BINARY}" GOOS=linux GOARCH="$GOARCH" DEBUG_BINARY=1
  repo="${repo}-debug"

  # build locally for integration test (the -l switch)
  bash scripts/build-upload-a-docker-image.sh -l -b -c "${BINARY}-debug" -d "cmd/${BINARY}" -t debug
  run_integration_test "localhost:5000/$repo"

  # build & upload official image
  bash scripts/build-upload-a-docker-image.sh ${LOCAL_FLAG} -b -c "${BINARY}-debug" -d "cmd/${BINARY}" -t debug
fi
