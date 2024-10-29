#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euf -o pipefail

print_help() {
  echo "Usage: $0 [-h] [-l] [-o] [-p platforms] [-b binary]"
  echo "-h: Print help"
  echo "-l: Enable local-only mode that only pushes images to local registry"
  echo "-o: overwrite image in the target remote repository even if the semver tag already exists"
  echo "-p: Comma-separated list of platforms to build for (default: all supported)"
  echo "-b: Which binary to build: 'all-in-one' (v1) (default) or 'jaeger' (v2)"
  exit 1
}

docker_compose_file="./examples/hotrod/docker-compose.yml"
platforms="$(make echo-linux-platforms)"
current_platform="$(go env GOOS)/$(go env GOARCH)"
BINARY="all-in-one"
FLAGS=()
success="false"

while getopts "hlop:b:" opt; do
	case "${opt}" in
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
	b)
    BINARY=${OPTARG}
		;;
	*)
		print_help
		;;
	esac
done

if [[ "$BINARY" != "all-in-one" && "$BINARY" != "jaeger" ]]; then
  echo "Invalid binary type provided: $BINARY"
  print_help
elif [[ "$BINARY" == "jaeger" ]]; then
  docker_compose_file="./examples/hotrod/docker-compose-v2.yml"
fi

set -x

dump_logs() {
  local compose_file=$1
  echo "::group:: Hotrod logs"
  docker compose -f "${compose_file}" logs
  echo "::endgroup::"
}

teardown() {
  echo "Tearing down..."
  if [[ "$success" == "false" ]]; then
    dump_logs "${docker_compose_file}"
  fi
  docker compose -f "$docker_compose_file" down
}
trap teardown EXIT

make prepare-docker-buildx
make create-baseimg LINUX_PLATFORMS="$platforms"

# Build hotrod binary for each target platform (separated by commas)
for platform in $(echo "$platforms" | tr ',' ' '); do
  # Extract the operating system from the platform string
  os=${platform%%/*}  #remove everything after the last slash
  # Extract the architecture from the platform string
  arch=${platform##*/}  # Remove everything before the last slash
  make build-examples GOOS="${os}" GOARCH="${arch}"
done

# Build hotrod image locally (-l) for integration test.
# Note: hotrod's Dockerfile is different from main binaries,
# so we do not pass flags like -b and -t.
bash scripts/build-upload-a-docker-image.sh -l -c example-hotrod -d examples/hotrod -p "${current_platform}"

# Build all-in-one (for v1) or jaeger (for v2) image locally (-l) for integration test
make build BINARY="$BINARY"
bash scripts/build-upload-a-docker-image.sh -l -b -c all-in-one -d cmd/all-in-one -p "${current_platform}" -t release

echo '::group:: docker compose'
JAEGER_VERSION=$GITHUB_SHA REGISTRY="localhost:5000/" docker compose -f "$docker_compose_file" up -d
echo '::endgroup::'

i=0
while [[ "$(curl -s -o /dev/null -w '%{http_code}' localhost:8080)" != "200" && $i -lt 30 ]]; do
  sleep 1
  i=$((i+1))
done

echo '::group:: check HTML'
echo 'Check that home page contains text Rides On Demand'
body=$(curl localhost:8080)
if [[ $body != *"Rides On Demand"* ]]; then
  echo "String \"Rides On Demand\" is not present on the index page"
  exit 1
fi
echo '::endgroup::'

response=$(curl -i -X POST "http://localhost:8080/dispatch?customer=123")
TRACE_ID=$(echo "$response" | grep -Fi "Traceresponse:" | awk '{print $2}' | cut -d '-' -f 2)

if [ -n "$TRACE_ID" ]; then
  echo "TRACE_ID is not empty: $TRACE_ID"
else
  echo "TRACE_ID is empty"
  exit 1
fi

JAEGER_QUERY_URL="http://localhost:16686"
EXPECTED_SPANS=35
MAX_RETRIES=30
SLEEP_INTERVAL=3

poll_jaeger() {
  local trace_id=$1
  local url="${JAEGER_QUERY_URL}/api/traces/${trace_id}"

  curl -s "${url}" | jq '.data[0].spans | length' || echo "0"
}

# Poll Jaeger until trace with desired number of spans is loaded or we timeout.
span_count=0
for ((i=1; i<=MAX_RETRIES; i++)); do
  span_count=$(poll_jaeger "${TRACE_ID}")

  if [[ "$span_count" -ge "$EXPECTED_SPANS" ]]; then
    echo "Trace found with $span_count spans."
    break
  fi

  echo "Retry $i/$MAX_RETRIES: Trace not found or insufficient spans ($span_count/$EXPECTED_SPANS). Retrying in $SLEEP_INTERVAL seconds..."
  sleep $SLEEP_INTERVAL
done

if [[ "$span_count" -lt "$EXPECTED_SPANS" ]]; then
  echo "Failed to find the trace with the expected number of spans within the timeout period."
  exit 1
fi

success="true"

# Ensure the image is published after successful test (maybe with -l flag if on a pull request).
# This is where all those multi-platform binaries we built earlier are utilized.
bash scripts/build-upload-a-docker-image.sh "${FLAGS[@]}" -c example-hotrod -d examples/hotrod -p "${platforms}"
