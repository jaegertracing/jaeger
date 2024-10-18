#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euf -o pipefail

JAEGER_QUERY_URL="http://localhost:16686"
EXPECTED_SPANS=35
MAX_RETRIES=30
SLEEP_INTERVAL=3

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

dump_logs() {
  local compose_file=$1
  echo "::group:: Hotrod logs"
  docker compose -f "${compose_file}" logs
  echo "::endgroup::"
}

teardown() {
  if [[ "${TEST_MODE:-}" == "docker" ]]; then
    teardown_docker
  elif [[ "${TEST_MODE:-}" == "k8s" ]]; then
    teardown_k8s
  else
    teardown_docker
    teardown_k8s
  fi
}

teardown_docker() {
  echo "Tearing down Docker..."
  if [[ "$success" == "false" ]]; then
    dump_logs "${docker_compose_file}"
  fi
  docker compose -f "$docker_compose_file" down
}

teardown_k8s() {
  echo "Cleaning up Kubernetes resources..."
  if [[ -n "${HOTROD_PORT_FWD_PID:-}" ]]; then
    kill "$HOTROD_PORT_FWD_PID" || true
  fi
  if [[ -n "${JAEGER_PORT_FWD_PID:-}" ]]; then
    kill "$JAEGER_PORT_FWD_PID" || true
  fi
  kubectl delete namespace example-hotrod --ignore-not-found=true
}

build_setup() {
  local platforms=$1
  
  make prepare-docker-buildx
  make create-baseimg LINUX_PLATFORMS="$platforms"
  
  # Build hotrod binary for each target platform
  for platform in $(echo "$platforms" | tr ',' ' '); do
    local os=${platform%%/*}
    local arch=${platform##*/}
    make build-examples GOOS="${os}" GOARCH="${arch}"
  done
  
  # Build local images for testing
  bash scripts/build-upload-a-docker-image.sh -l -c example-hotrod -d examples/hotrod -p "${current_platform}"
  make build-all-in-one
  bash scripts/build-upload-a-docker-image.sh -l -b -c all-in-one -d cmd/all-in-one -p "${current_platform}" -t release
}

check_web_app() {
  local i=0
  while [[ "$(curl -s -o /dev/null -w '%{http_code}' localhost:8080)" != "200" && $i -lt 30 ]]; do
    sleep 1
    i=$((i+1))
  done
  
  if [[ $i -eq 30 ]]; then
    echo "Timeout waiting for web app to become available"
    exit 1
  fi
}

verify_home_page() {
  echo '::group:: check HTML'
  echo 'Check that home page contains text Rides On Demand'
  local body
  body=$(curl -s localhost:8080)
  if [[ $body != *"Rides On Demand"* ]]; then
    echo "String \"Rides On Demand\" is not present on the index page"
    exit 1
  fi
  echo '::endgroup::'
}

get_trace_id() {
  local response
  local trace_id
  
  response=$(curl -s -i -X POST "http://localhost:8080/dispatch?customer=123")
  trace_id=$(echo "$response" | grep -Fi "Traceresponse:" | awk '{print $2}' | cut -d '-' -f 2)
  
  if [ -z "$trace_id" ]; then
    echo "Failed to get trace ID" >&2
    exit 1
  fi
  
  echo "$trace_id"
}

verify_trace() {
  local trace_id=$1
  local span_count=0
  
  for ((i=1; i<=MAX_RETRIES; i++)); do
    span_count=$(curl -s "${JAEGER_QUERY_URL}/api/traces/${trace_id}" | jq '.data[0].spans | length' || echo "0")
    
    if [[ "$span_count" -ge "$EXPECTED_SPANS" ]]; then
      echo "Trace found with $span_count spans."
      return 0
    fi
    
    echo "Retry $i/$MAX_RETRIES: Found $span_count/$EXPECTED_SPANS spans. Retrying in $SLEEP_INTERVAL seconds..."
    sleep $SLEEP_INTERVAL
  done
  
  echo "Failed to find trace with expected number of spans within timeout period"
  return 1
}

run_docker_tests() {
  TEST_MODE="docker"
  echo "Running Docker tests..."
  
  JAEGER_VERSION=$GITHUB_SHA REGISTRY="localhost:5000/" docker compose -f "$docker_compose_file" up -d
  
  check_web_app
  verify_home_page
  
  local trace_id
  trace_id=$(get_trace_id)
  verify_trace "$trace_id"
}

verify_cluster() {
    echo "Verifying cluster connection..."
    if ! kubectl cluster-info >/dev/null 2>&1; then
        echo "Error: Cannot connect to Kubernetes cluster"
        echo "Please check:"
        echo "  1. Cluster is running (minikube status)"
        echo "  2. kubectl is properly configured (kubectl config current-context)"
        return 1
    fi
}

run_k8s_tests() {
  TEST_MODE="k8s"
  echo "Running Kubernetes tests..."

  verify_cluster || exit 1
  
  # Create namespace and deploy resources
  kubectl create namespace example-hotrod --dry-run=client -o yaml | kubectl apply --validate-false -f -
  kustomize build ./examples/hotrod/kubernetes | kubectl apply -n example-hotrod -f -
  
  # Wait for deployments
  kubectl wait --for=condition=available --timeout=180s -n example-hotrod deployment/example-hotrod
  kubectl wait --for=condition=available --timeout=180s -n example-hotrod deployment/jaeger
  
  # Setup port forwarding
  kubectl port-forward -n example-hotrod svc/example-hotrod 8080:8080 &
  HOTROD_PORT_FWD_PID=$!
  kubectl port-forward -n example-hotrod svc/jaeger 16686:16686 &
  JAEGER_PORT_FWD_PID=$!
  
  # Wait for port-forward to be ready
  sleep 5
  
  check_web_app
  verify_home_page
  
  local trace_id
  trace_id=$(get_trace_id)
  verify_trace "$trace_id"
}

main() {
  docker_compose_file="./examples/hotrod/docker-compose.yml"
  platforms="$(make echo-linux-platforms)"
  current_platform="$(go env GOOS)/$(go env GOARCH)"
  FLAGS=()
  success="false"
  
  trap teardown EXIT
  
  local run_docker=true
  local run_kubernetes=true
  
  while getopts "hlop:dk" opt; do
    case "${opt}" in
      l) FLAGS=("${FLAGS[@]}" -l) ;;
      o) FLAGS=("${FLAGS[@]}" -o) ;;
      p) platforms=${OPTARG} ;;
      d) run_kubernetes=false ;;
      k) run_docker=false ;;
      *) print_help ;;
    esac
  done
  
  build_setup "$platforms"
  
  if [[ "$run_docker" == "true" ]]; then
    run_docker_tests
  fi
  
  if [[ "$run_kubernetes" == "true" ]]; then
    run_k8s_tests
  fi
  
  success="true"
  
  # Build and upload the final image
  bash scripts/build-upload-a-docker-image.sh "${FLAGS[@]}" -c example-hotrod -d examples/hotrod -p "${platforms}"
}

main "$@"