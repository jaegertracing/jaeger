#!/bin/bash

set -euxf -o pipefail

# Constants
BINARY='example-hotrod'
REPO="jaegertracing/${BINARY}"
PLATFORMS="linux/amd64,linux/s390x,linux/ppc64le,linux/arm64"
LOCAL_FLAG='-l'

# Function to run integration tests
run_integration_test() {
  local image_name="$1"
  export CID
  CID=$(docker run -d --name "${BINARY}" -p 8080:8080 "${image_name}:${GITHUB_SHA}")

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
  docker rm -f "$CID"
}

# Build for each platform
for platform in $(echo "$PLATFORMS" | tr ',' ' '); do
  os=${platform%/*}   # Extract OS (part before the slash)
  arch=${platform##*/} # Extract architecture (part after the slash)
  make build-examples GOOS="${os}" GOARCH="${arch}"
done

# Build image locally for integration test
bash scripts/build-upload-a-docker-image.sh -l -c "${BINARY}" -d "${BINARY}" -p "linux/amd64"

# Run integration test on the local image
run_integration_test "localhost:5000/${REPO}"

# Build and upload image to dockerhub/quay.io
bash scripts/build-upload-a-docker-image.sh ${LOCAL_FLAG} -c "${BINARY}" -d "${BINARY}" -p "${PLATFORMS}"

# Build debug image if requested
if [[ "${add_debugger}" == "Y" ]]; then
  for platform in $(echo "$PLATFORMS" | tr ',' ' '); do
    os=${platform%/*}
    arch=${platform##*/}
    make build-examples GOOS="${os}" GOARCH="${arch}" DEBUG_BINARY=1
  done
  repo="${REPO}-debug"

  # Build locally for integration test
  bash scripts/build-upload-a-docker-image.sh -l -c "${BINARY}-debug" -d "${BINARY}" -t debug
  run_integration_test "localhost:5000/$repo"

  # Build and upload official debug image
  bash scripts.build-upload-a-docker-image.sh ${LOCAL_FLAG} -c "${BINARY}-debug" -d "${BINARY}" -t debug
fi
