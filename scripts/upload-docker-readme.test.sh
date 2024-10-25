#!/bin/bash

set -euo pipefail

# Mock variables
dockerhub_repository="jaegertracing/test-docker"
http_code=""
response_body=""

simulate_update() {
  local status=$1
  if [ "$status" -eq 200 ]; then
    http_code=200
    response_body="Successfully updated README."
  else
    http_code="$status"
    response_body="Error: Unable to update README."
  fi
}

# Test 1: Successful update
echo "Running test: Successful update"
simulate_update 200
if [ "$http_code" -eq 200 ]; then
  echo "Successfully updated Docker Hub README for $dockerhub_repository"
else
  echo "Failed to update Docker Hub README for $dockerhub_repository with status code $http_code"
  echo "Full response: $response_body"
fi

# Test 2: Failed update
echo "Running test: Failed update"
simulate_update 403
if [ "$http_code" -eq 200 ]; then
  echo "Successfully updated Docker Hub README for $dockerhub_repository"
else
  echo "Failed to update Docker Hub README for $dockerhub_repository with status code $http_code"
  echo "Full response: $response_body"
fi
