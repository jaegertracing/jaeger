#!/bin/bash

set -euxf -o pipefail

docker_compose_file="./examples/hotrod/docker-compose.yml"
platforms="linux/amd64,linux/s390x,linux/ppc64le,linux/arm64"

teardown() {
  echo "Tearing down..."
  docker compose -f "$docker_compose_file" down
}
trap teardown EXIT

make build-examples GOOS=linux GOARCH=amd64
make build-examples GOOS=linux GOARCH=s390x
make build-examples GOOS=linux GOARCH=ppc64le
make build-examples GOOS=linux GOARCH=arm64

make prepare-docker-buildx
make create-baseimg

# Loop through each platform (separated by commas)
for platform in $(echo "$platforms" | tr ',' ' '); do
  # Extract the architecture from the platform string
  arch=${platform##*/}  # Remove everything before the last slash
  make "build-all-in-one" GOOS=linux GOARCH="${arch}"
done

# Build image locally (-l) for integration test
bash scripts/build-upload-a-docker-image.sh -l -c example-hotrod -d examples/hotrod -p "${platforms}"
bash scripts/build-upload-a-docker-image.sh -l -b -c all-in-one -d cmd/all-in-one -p "${platforms}" -t release

JAEGER_VERSION=$GITHUB_SHA REGISTRY="localhost:5000/" docker compose -f "$docker_compose_file" up -d

i=0
while [[ "$(curl -s -o /dev/null -w '%{http_code}' localhost:8080)" != "200" && $i -lt 30 ]]; do
  sleep 1
  i=$((i+1))
done

body=$(curl localhost:8080)
if [[ $body != *"Rides On Demand"* ]]; then
  echo "String \"Rides On Demand\" is not present on the index page"
  exit 1
fi

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
SLEEP_INTERVAL=10

# Function to poll Jaeger for the trace
poll_jaeger() {
  local trace_id=$1
  local url="${JAEGER_QUERY_URL}/api/traces/${trace_id}"

  curl -s "${url}" | jq '.data[0].spans | length' || echo "0"
}

# Polling loop
for ((i=1; i<=MAX_RETRIES; i++)); do
  span_count=$(poll_jaeger "${TRACE_ID}")

  if [[ "$span_count" -ge "$EXPECTED_SPANS" ]]; then
    echo "Trace found with $span_count spans."
    exit 0
  fi

  echo "Retry $i/$MAX_RETRIES: Trace not found or insufficient spans ($span_count/$EXPECTED_SPANS). Retrying in $SLEEP_INTERVAL seconds..."
  sleep $SLEEP_INTERVAL
done

echo "Failed to find the trace with the expected number of spans within the timeout period."
exit 1
