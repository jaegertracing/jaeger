#!/bin/bash

set -euxf -o pipefail

make build-examples GOOS=linux GOARCH=amd64
make build-examples GOOS=linux GOARCH=s390x
make build-examples GOOS=linux GOARCH=ppc64le
make build-examples GOOS=linux GOARCH=arm64

REPO=jaegertracing/example-hotrod
platforms="linux/amd64,linux/s390x,linux/ppc64le,linux/arm64"
make prepare-docker-buildx
#build image locally for integration test
bash scripts/build-upload-a-docker-image.sh -l -c example-hotrod -d examples/hotrod -p "${platforms}"

export CID=$(docker run -d -p 8080:8080 localhost:5000/$REPO:latest)
i=0
while [[ "$(curl -s -o /dev/null -w ''%{http_code}'' localhost:8080)" != "200" && ${i} -lt 30 ]]; do
  sleep 1
  i=$((i+1))
done
body=$(curl localhost:8080)
if [[ $body != *"Rides On Demand"* ]]; then
  echo "String \"Rides On Demand\" is not present on the index page"
  exit 1
fi
docker rm -f $CID

BRANCH=${BRANCH:?'missing BRANCH env var'}
bash scripts/build-upload-a-docker-image.sh -c example-hotrod -d examples/hotrod -p "${platforms}"
