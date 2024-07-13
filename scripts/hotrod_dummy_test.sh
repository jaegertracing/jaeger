#!/bin/bash

set -euxf -o pipefail

make build-examples GOOS=linux GOARCH=amd64
make build-examples GOOS=linux GOARCH=s390x
make build-examples GOOS=linux GOARCH=ppc64le
make build-examples GOOS=linux GOARCH=arm64

REPO=jaegertracing/example-hotrod
platforms="linux/amd64,linux/s390x,linux/ppc64le,linux/arm64"
make prepare-docker-buildx

# build image locally (-l) for integration test
bash scripts/build-upload-a-docker-image.sh -l -c example-hotrod -d examples/hotrod -p "${platforms}"


docker compose -f ./docker-compose/hotrod/docker-compose.yml up -d

#curl frontend-endpoint
docker compose -f ./docker-compose/hotrod/docker-compose.yml logs hotrod