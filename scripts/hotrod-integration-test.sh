#!/bin/bash

set -euxf -o pipefail

print_help() {
  echo "Usage: $0 [-b binary] [-D] [-l] [-p platforms]"
  echo "-b: Which binary to build (default: 'example-hotrod')"
  echo "-D: Disable building of images with debugger"
  echo "-h: Print help"
  echo "-l: Enable local-only mode that only pushes images to local registry"
  echo "-p: Comma-separated list of platforms to build for (default: all supported)"
  exit 1
}

add_debugger='Y'
platforms="linux/amd64,linux/s390x,linux/ppc64le,linux/arm64"
LOCAL_FLAG=''
BINARY='example-hotrod'

while getopts "b:Dhlp:" opt; do
  case "${opt}" in
    b)
      BINARY=${OPTARG}
      ;;
    D)
      add_debugger='N'
      ;;
    l)
      LOCAL_FLAG='-l'
      ;;
    p)
      platforms=${OPTARG}
      ;;
    ?)
      print_help
      ;;
  esac
done

set -x

REPO=jaegertracing/${BINARY}
make prepare-docker-buildx

for platform in $(echo "$platforms" | tr ',' ' '); do
  arch=${platform##*/}
  make build-examples GOOS=linux GOARCH="${arch}"
done

# build image locally for integration test (the -l switch)
bash scripts/build-upload-a-docker-image.sh -l -c "${BINARY}" -d examples/hotrod -p "${platforms}"

# pass --name example-hotrod so that we can do `docker logs example-hotrod` later
export CID
CID=$(docker run -d --name example-hotrod -p 8080:8080 "localhost:5000/${REPO}:${GITHUB_SHA}")

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

# build and upload image to dockerhub/quay.io
bash scripts/build-upload-a-docker-image.sh ${LOCAL_FLAG} -c "${BINARY}" -d examples/hotrod -p "${platforms}"

# build debug image if requested
if [[ "${add_debugger}" == "Y" ]]; then
  make build-examples GOOS=linux GOARCH="${GOARCH}" DEBUG_BINARY=1
  repo="${REPO}-debug"

  # build locally for integration test (the -l switch)
  bash scripts/build-upload-a-docker-image.sh -l -c "${BINARY}-debug" -d examples/hotrod -t debug
  run_integration_test "localhost:5000/$repo"

  # build & upload official image
  bash scripts/build-upload-a-docker-image.sh ${LOCAL_FLAG} -c "${BINARY}-debug" -d examples/hotrod -t debug
fi
