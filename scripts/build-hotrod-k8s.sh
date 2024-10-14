#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

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

# Function to run the Kubernetes integration test
run_integration_tests() {
  echo "Running Kubernetes integration tests..."
  
  # Check HotROD homepage
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

  # Dispatch a request
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
}

# Function to clean up the deployed resources
cleanup_k8s_resources() {
  echo "Cleaning up resources..."
  kill $HOTROD_PORT_FWD_PID
  kill $JAEGER_PORT_FWD_PID
  kustomize build /examples/hotrod/kubernetes | kubectl delete -f -
}