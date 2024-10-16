#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euf -o pipefail

# Prints usage information
print_help() {
  echo "Usage: $0 [-h] [-l] [-o] [-p platforms] [-d | -k]"
  echo "-h: Print help"
  echo "-l: Enable local-only mode that only pushes images to local registry"
  echo "-o: Overwrite image in the target remote repository even if the semver tag already exists"
  echo "-p: Comma-separated list of platforms to build for (default: all supported)"
  echo "-d: Run Docker tests only"
  echo "-k: Run Kubernetes tests only"
  exit 1
}

# Dumps the logs for docker compose
dump_logs() {
  local compose_file=$1
  echo "::group:: Hotrod logs"
  docker compose -f "${compose_file}" logs
  echo "::endgroup::"
}

# Teardown logic for Docker
teardown_docker() {
  echo "Tearing down Docker..."
  if [[ "$success" == "false" ]]; then
    dump_logs "${docker_compose_file}"
  fi
  docker compose -f "$docker_compose_file" down
}

# Teardown logic for Kubernetes
teardown_k8s() {
  echo "Cleaning up Kubernetes resources..."
  kill $HOTROD_PORT_FWD_PID || true
  kill $JAEGER_PORT_FWD_PID || true
  kustomize build /examples/hotrod/kubernetes | kubectl delete -f - || true
}

# Function to build binaries for the given platforms
build_binaries_for_platforms() {
  local platforms=$1
  for platform in $(echo "$platforms" | tr ',' ' '); do
    os=${platform%%/*}   # Extract OS
    arch=${platform##*/} # Extract architecture
    make build-examples GOOS="${os}" GOARCH="${arch}"
  done
}

# Function to run Docker tests
run_docker_tests() {
  echo "Running Docker tests..."

  run_docker_compose

  check_web_app
  check_home_page

  TRACE_ID=$(get_trace_id)

  wait_for_trace "$TRACE_ID"

  success="true"
}

# Sets up docker-compose and waits for services to be ready
run_docker_compose() {
  echo '::group:: docker compose'
  JAEGER_VERSION=$GITHUB_SHA REGISTRY="localhost:5000/" docker compose -f "$docker_compose_file" up -d
  echo '::endgroup::'
}

# Checks if the web app is running
check_web_app() {
  i=0
  while [[ "$(curl -s -o /dev/null -w '%{http_code}' localhost:8080)" != "200" && $i -lt 30 ]]; do
    sleep 1
    i=$((i+1))
  done
}

# Verifies if the hotrod app's home page contains the expected string
check_home_page() {
  echo '::group:: check HTML'
  echo 'Check that home page contains text Rides On Demand'
  body=$(curl localhost:8080)
  if [[ $body != *"Rides On Demand"* ]]; then
    echo "String \"Rides On Demand\" is not present on the index page"
    exit 1
  fi
  echo '::endgroup::'
}

# Extracts the trace ID from the POST response
get_trace_id() {
  local response=$(curl -i -X POST "http://localhost:8080/dispatch?customer=123")
  local trace_id=$(echo "$response" | grep -Fi "Traceresponse:" | awk '{print $2}' | cut -d '-' -f 2)
  if [ -n "$trace_id" ]; then
    echo "TRACE_ID is not empty: $trace_id"
  else
    echo "TRACE_ID is empty"
    exit 1
  fi
  echo "$trace_id"
}

# Polls Jaeger for trace data
poll_jaeger() {
  local trace_id=$1
  local url="${JAEGER_QUERY_URL}/api/traces/${trace_id}"
  curl -s "${url}" | jq '.data[0].spans | length' || echo "0"
}

# Waits until Jaeger trace contains the expected number of spans
wait_for_trace() {
  local trace_id=$1
  local span_count=0

  for ((i=1; i<=MAX_RETRIES; i++)); do
    span_count=$(poll_jaeger "${trace_id}")
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
}

# Deploy Kubernetes resources for HotROD and Jaeger
deploy_k8s_resources() {
  echo "Deploying HotROD and Jaeger to Kubernetes..."
  kustomize build /examples/hotrod/kubernetes | kubectl apply -f -

  # Wait for services to be ready
  kubectl wait --for=condition=available --timeout=180s deployment/example-hotrod -n example-hotrod
  kubectl wait --for=condition=available --timeout=180s deployment/jaeger -n example-hotrod

  # Port-forward HotROD and Jaeger services for local access
  kubectl port-forward -n example-hotrod svc/example-hotrod 8080:frontend &
  HOTROD_PORT_FWD_PID=$!
  kubectl port-forward -n example-hotrod svc/jaeger 16686:frontend &
  JAEGER_PORT_FWD_PID=$!
}

# Run Kubernetes integration tests
run_k8s_tests() {
  echo "Running Kubernetes tests..."
  
  deploy_k8s_resources
  check_web_app
  check_home_page

  TRACE_ID=$(get_trace_id)
  wait_for_trace "$TRACE_ID"

  success="true"
}

# Main script execution
main() {
  docker_compose_file="./examples/hotrod/docker-compose.yml"
  platforms="$(make echo-linux-platforms)"
  current_platform="$(go env GOOS)/$(go env GOARCH)"
  FLAGS=()
  success="false"

  while getopts "hlop:dk" opt; do
    case "${opt}" in
      l)
        FLAGS=("${FLAGS[@]}" -l)
        ;;
      o)
        FLAGS=("${FLAGS[@]}" -o)
        ;;
      p)
        platforms=${OPTARG}
        ;;
      d)
        run_docker_tests
        exit 0
        ;;
      k)
        run_k8s_tests
        exit 0
        ;;
      *)
        print_help
        ;;
    esac
  done

  # If no specific option was provided, run both Docker and Kubernetes tests
  make prepare-docker-buildx
  make create-baseimg LINUX_PLATFORMS="$platforms"
  build_binaries_for_platforms "$platforms"
  
  run_docker_tests
  run_k8s_tests

  success="true"
}

main "$@"
