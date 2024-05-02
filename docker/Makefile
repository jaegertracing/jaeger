VERSION := 1.0.0
ROOT_IMAGE ?= alpine:3.16
CERT_IMAGE := $(ROOT_IMAGE)
GOLANG_IMAGE := golang:1.22-alpine

DOCKER_REGISTRY ?= localhost:5000
BASE_IMAGE ?= $(DOCKER_REGISTRY)/baseimg_alpine:latest
DEBUG_IMAGE ?= $(DOCKER_REGISTRY)/debugimg_alpine:latest
PLATFORMS ?= linux/amd64,linux/s390x,linux/ppc64le,linux/arm64

create-baseimg-debugimg: create-baseimg create-debugimg

create-baseimg: prepare-docker-buildx
ifeq ($(shell git rev-parse --abbrev-ref HEAD), main)
  # Build for all architectures on main branch
  docker buildx build -t $(BASE_IMAGE) --push \
    --build-arg root_image=$(ROOT_IMAGE) \
    --build-arg cert_image=$(CERT_IMAGE) \
    --platform=$(PLATFORMS) \
    docker/base
else
  # Build only for linux/amd64 on non-main branches (pull requests)
  docker buildx build -t $(BASE_IMAGE) --push \
    --build-arg root_image=$(ROOT_IMAGE) \
    --build-arg cert_image=$(CERT_IMAGE) \
    --platform=linux/amd64 \
    docker/base
fi

.PHONY: prepare-docker-buildx
prepare-docker-buildx:
  docker buildx inspect jaeger-build > /dev/null || docker buildx create --use --name=jaeger-build --buildkitd-flags="--allow-insecure-entitlement security.insecure --allow-insecure-entitlement network.host" --driver-opt="network=host"
  docker inspect registry > /dev/null || docker run --rm -d -p 5000:5000 --name registry registry:2

.PHONY: clean-docker-buildx
clean-docker-buildx:
  docker buildx rm jaeger-build
  docker rm -f registry
