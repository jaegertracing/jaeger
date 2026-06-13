#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euf -o pipefail

print_help() {
  echo "Usage: $0 [-D] [-h] [-l] [-o] [-p platforms]"
  echo "  -D: Disable building of images with debugger"
  echo "  -h: Print help"
  echo "  -l: Enable local-only mode that only pushes images to local registry"
  echo "  -o: overwrite image in the target remote repository even if the semver tag already exists"
  echo "  -p: Comma-separated list of platforms to build for (default: all supported)"
  exit 1
}

add_debugger='Y'
platforms="$(make echo-linux-platforms)"
FLAGS=()
BINARY="jaeger"

# this script doesn't use BRANCH and GITHUB_SHA itself, but its dependency scripts do.
export BRANCH=${BRANCH?'env var is required'}
export GITHUB_SHA=${GITHUB_SHA:-$(git rev-parse HEAD)}

while getopts "Dhlop:" opt; do
	case "${opt}" in
	D)
		add_debugger='N'
		;;
	l)
		# in the local-only mode the images will only be pushed to local registry
		FLAGS=("${FLAGS[@]}" -l)
		;;
	o)
		FLAGS=("${FLAGS[@]}" -o)
		;;
	p)
		platforms=${OPTARG}
		;;
	?)
		print_help
		;;
	esac
done

# remove flags, leave only positional args
shift $((OPTIND - 1))

# Only build the jaeger binary
export HEALTHCHECK_V2=true

set -x

# Set default GOARCH variable to the host GOARCH, the target architecture can
# be overrided by passing architecture value to the script:
# `GOARCH=<target arch> ./scripts/build/build-all-in-one-image.sh`.
GOARCH=${GOARCH:-$(go env GOARCH)}
image="jaegertracing/${BINARY}"

make build-ui

run_integration_test() {
  local image_name="$1"
  CID=$(docker run -d -p 16686:16686 -p 13133:13133 -p 5778:5778 "${image_name}:${GITHUB_SHA}")

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

build_test_upload() {
  # Loop through each platform (separated by commas)
  for platform in $(echo "$platforms" | tr ',' ' '); do
    arch=${platform##*/}  # Remove everything before the last slash
    make "build-${BINARY}" GOOS=linux GOARCH="${arch}"
  done

  make create-baseimg LINUX_PLATFORMS="$platforms"

  # build all-in-one image locally for integration test (the explicit -l switch)
  bash scripts/build/build-upload-a-docker-image.sh -l -b -c "${BINARY}" -d "cmd/${BINARY}" -p "${platforms}" -t release

  run_integration_test "localhost:5000/$image"

  # build all-in-one image and upload to dockerhub/quay.io
  bash scripts/build/build-upload-a-docker-image.sh "${FLAGS[@]}" -b -c "${BINARY}" -d "cmd/${BINARY}" -p "${platforms}" -t release
}

build_test_upload_with_debugger() {
  make "build-${BINARY}" GOOS=linux GOARCH="$GOARCH" DEBUG_BINARY=1

  make create-baseimg-debugimg LINUX_PLATFORMS="$platforms"

  # build locally for integration test (the -l switch)
  bash scripts/build/build-upload-a-docker-image.sh -l -b -c "${BINARY}-debug" -d "cmd/${BINARY}" -p "${platforms}" -t release -t debug
  run_integration_test "localhost:5000/${image}-debug"

  # build & upload official image
  bash scripts/build/build-upload-a-docker-image.sh "${FLAGS[@]}" -b -c "${BINARY}-debug" -d "cmd/${BINARY}" -p "${platforms}" -t debug
}

build_test_upload

if [[ "${add_debugger}" == "Y" ]]; then
  build_test_upload_with_debugger
fi
