#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euf -o pipefail

print_help() {
  echo "Usage: $0 [-h] [-l] [-o] [-p platforms] [-v jaeger_version]"
  echo "-h: Print help"
  echo "-l: Enable local-only mode that only pushes images to local registry"
  echo "-o: overwrite image in the target remote repository even if the semver tag already exists"
  echo "-p: Comma-separated list of platforms to build for (default: all supported)"
  echo "-v: Jaeger version to use for hotrod image (v1 or v2, default: v2)"
  echo "-r: Runtime to test with (docker|k8s, default: docker)"
  exit 1
}

docker_compose_file="./examples/hotrod/docker-compose-v2.yml"
platforms="$(make echo-linux-platforms)"
current_platform="$(go env GOOS)/$(go env GOARCH)"
jaeger_version="v2"
binary="jaeger"
FLAGS=()
success="false"
runtime="docker"

while getopts "hlop:v:r:" opt; do
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
	v)
		jaeger_version=${OPTARG}
		;;
  r)
		case "${OPTARG}" in
			docker|k8s) runtime="${OPTARG}" ;;
			*) echo "Invalid runtime: ${OPTARG}. Use 'docker' or 'k8s'" >&2; exit 1 ;;
		esac
		;;
	*)
		print_help
		;;
	esac
done

case "$jaeger_version" in
  v1)
    docker_compose_file="./examples/hotrod/docker-compose-v1.yml"
    binary="all-in-one"
    ;;
  v2)
    docker_compose_file="./examples/hotrod/docker-compose-v2.yml"
    binary="jaeger"
    ;;
  *)
    echo "Invalid Jaeger version provided: $jaeger_version"
    print_help
    ;;
esac

set -x

dump_logs() {
  local runtime=$1
  local compose_file=$2

  echo "::group:: Logs"
  if [ "$runtime" == "k8s" ]; then
    kubectl logs -n example-hotrod -l app=example-hotrod
    kubectl logs -n example-hotrod -l app=jaeger
  else
    docker compose -f "$compose_file" logs
  fi
  echo "::endgroup::"
}

teardown() {
  echo "::group::Tearing down..."
  if [[ "$success" == "false" ]]; then
      dump_logs "${runtime}" "${docker_compose_file}"
  fi
  if [[ "${runtime}" == "k8s" ]]; then
    if [[ -n "${HOTROD_PORT_FWD_PID:-}" ]]; then
      kill "$HOTROD_PORT_FWD_PID" || true
    fi
    if [[ -n "${JAEGER_PORT_FWD_PID:-}" ]]; then
      kill "$JAEGER_PORT_FWD_PID" || true
    fi
    kubectl delete namespace example-hotrod --ignore-not-found=true
  else
    docker compose -f "$docker_compose_file" down
  fi
  
  echo "::endgroup::"
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
bash scripts/build/build-upload-a-docker-image.sh -l -c example-hotrod -d examples/hotrod -p "${current_platform}"

# Build all-in-one (for v1) or jaeger (for v2) image locally (-l) for integration test
make build-${binary}
bash scripts/build/build-upload-a-docker-image.sh -l -b -c "${binary}" -d cmd/"${binary}" -p "${current_platform}" -t release

if [[ "${runtime}" == "k8s" ]]; then
  if ! kubectl cluster-info >/dev/null 2>&1; then
    echo "Error: Cannot connect to Kubernetes cluster"
    exit 1
  fi

  echo '::group:: run on Kubernetes'
  kustomize build ./examples/hotrod/kubernetes | kubectl apply -n example-hotrod -f -
  kubectl wait --for=condition=available --timeout=180s -n example-hotrod deployment/example-hotrod
  
  kubectl port-forward -n example-hotrod service/example-hotrod 8080:frontend &
  HOTROD_PORT_FWD_PID=$!
  kubectl port-forward -n example-hotrod service/jaeger 16686:frontend &
  JAEGER_PORT_FWD_PID=$!
  echo '::endgroup::'

else
  echo '::group:: docker compose'
  JAEGER_VERSION=$GITHUB_SHA HOTROD_VERSION=$GITHUB_SHA REGISTRY="localhost:5000/" docker compose -f "$docker_compose_file" up -d
  echo '::endgroup::'
fi

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
bash scripts/build/build-upload-a-docker-image.sh "${FLAGS[@]}" -c example-hotrod -d examples/hotrod -p "${platforms}"
