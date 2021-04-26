#!/bin/bash

set -exu

VERSION=1.0.0
ROOT_IMAGE=alpine:3.12
CERT_IMAGE=alpine:3.12
BASE_IMAGE_MULTIARCH=localhost:5000/baseimg:${VERSION}-$(echo ${ROOT_IMAGE} | sed "s/:/-/")
PLATFORMS="linux/amd64,linux/s390x"

docker buildx build -t ${BASE_IMAGE_MULTIARCH} --push \
	--build-arg root_image=${ROOT_IMAGE} \
	--build-arg cert_image=${CERT_IMAGE} \
	--platform=${PLATFORMS} \
	docker/base
echo "Finished building multiarch base image =============="
