#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euf -o pipefail

print_help() {
  echo "Usage: $0 [-b binary] [-D] [-h] [-l] [-o] [-p platforms] <jaeger_version>"
  echo "  -D: Disable building of images with debugger"
  echo "  -h: Print help"
  echo "  -l: Enable local-only mode that only pushes images to local registry"
  echo "  -o: overwrite image in the target remote repository even if the semver tag already exists"
  echo "  -p: Comma-separated list of platforms to build for (default: all supported)"
  echo "  jaeger_version:  major version, v1 | v2"
  exit 1
}

add_debugger='Y'
platforms="$(make echo-linux-platforms)"
FLAGS=()

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

if [[ $# -eq 0 ]]; then
  echo "Jaeger major version is required as argument"
  print_help
fi

case $1 in
  v1)
    BINARY='all-in-one'
    sampling_port=14268
    export HEALTHCHECK_V2=false
    ;;
  v2)
    BINARY='jaeger'
    sampling_port=5778
    export HEALTHCHECK_V2=true
    ;;
  *)
    echo "Jaeger major version is required as argument"
    print_help
esac

set -x

# Set default GOARCH variable to the host GOARCH, the target architecture can
# be overrided by passing architecture value to the script:
# `GOARCH=<target arch> ./scripts/build/build-all-in-one-image.sh`.
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
  CID=$(docker run -d -p 16686:16686 -p 13133:13133 -p "14268:${sampling_port}" "${image_name}:${GITHUB_SHA}")

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
  arch=${platform##*/}  # Remove everything before the last slash
  make "build-${BINARY}" GOOS=linux GOARCH="${arch}"
done

baseimg_target='create-baseimg-debugimg'
if [[ "${add_debugger}" == "N" ]]; then
  baseimg_target='create-baseimg'
fi
make "$baseimg_target" LINUX_PLATFORMS="$platforms"

# build all-in-one image locally for integration test (the explicit -l switch)
bash scripts/build/build-upload-a-docker-image.sh -l -b -c "${BINARY}" -d "cmd/${BINARY}" -p "${platforms}" -t release

run_integration_test "localhost:5000/$repo"

# build all-in-one image and upload to dockerhub/quay.io
bash scripts/build/build-upload-a-docker-image.sh "${FLAGS[@]}" -b -c "${BINARY}" -d "cmd/${BINARY}" -p "${platforms}" -t release

# build debug image if requested
if [[ "${add_debugger}" == "Y" ]]; then
  make "build-${BINARY}" GOOS=linux GOARCH="$GOARCH" DEBUG_BINARY=1
  repo="${repo}-debug"

  # build locally for integration test (the -l switch)
  bash scripts/build/build-upload-a-docker-image.sh -l -b -c "${BINARY}-debug" -d "cmd/${BINARY}" -t debug
  run_integration_test "localhost:5000/$repo"

  # build & upload official image
  bash scripts/build/build-upload-a-docker-image.sh "${FLAGS[@]}" -b -c "${BINARY}-debug" -d "cmd/${BINARY}" -t debug
fi
