#!/bin/bash

set -euxf -o pipefail

make build-examples GOOS=linux GOARCH=amd64
make build-examples GOOS=linux GOARCH=s390x
make build-examples GOOS=linux GOARCH=ppc64le
make build-examples GOOS=linux GOARCH=arm64


platforms="linux/amd64,linux/s390x,linux/ppc64le,linux/arm64"
make prepare-docker-buildx

# build image locally (-l) for integration test
bash scripts/build-upload-a-docker-image.sh -l -c example-hotrod -d examples/hotrod -p "${platforms}"

export HOTROD_IMAGE="localhost:5000/jaegertracing/example-hotrod"
docker compose -f ./examples/hotrod/docker-compose.yml up -d

i=0
while [[ "$(curl -s -o /dev/null -w '%{http_code}' localhost:8080)" != "200" && ${i} -lt 30 ]]; do
  sleep 1
  i=$((i+1))
done
body=$(curl localhost:8080)
if [[ $body != *"Rides On Demand"* ]]; then
  echo "String \"Rides On Demand\" is not present on the index page"
  exit 1
fi

curl -X POST "http://localhost:8080/dispatch?customer=123"
# Extract the logs from the docker compose service
logs=$(docker compose -f ./examples/hotrod/docker-compose.yml logs hotrod)

# Extract the trace_id from the logs
TRACE_ID=$(echo "$logs" | grep -oP '(?<="trace_id": ")[^"]+' | tail -n 1)

JAEGER_QUERY_URL="http://localhost:16686"
EXPECTED_SPANS=10  # Change this to the expected number of spans
MAX_RETRIES=30     
SLEEP_INTERVAL=10  

# Function to poll Jaeger for the trace
poll_jaeger() {
  local trace_id=$1
  local url="${JAEGER_QUERY_URL}/api/traces/${trace_id}"

  curl -s "${url}" | jq '.data[0].spans | length'
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
