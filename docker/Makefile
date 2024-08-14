# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

VERSION := 1.0.0

DOCKER_REGISTRY ?= localhost:5000
BASE_IMAGE ?= $(DOCKER_REGISTRY)/baseimg_alpine:latest
DEBUG_IMAGE ?= $(DOCKER_REGISTRY)/debugimg_alpine:latest
PLATFORMS ?= linux/amd64,linux/s390x,linux/ppc64le,linux/arm64

create-baseimg-debugimg: create-baseimg create-debugimg

create-baseimg: prepare-docker-buildx
	docker buildx build -t $(BASE_IMAGE) --push \
		--platform=$(PLATFORMS) \
		docker/base

create-debugimg: prepare-docker-buildx
	docker buildx build -t $(DEBUG_IMAGE) --push \
		--platform=$(PLATFORMS) \
		docker/debug

create-fake-debugimg: prepare-docker-buildx
	docker buildx build -t $(DEBUG_IMAGE) --push \
		--platform=$(PLATFORMS) \
		docker/base

.PHONY: prepare-docker-buildx
prepare-docker-buildx:
	docker buildx inspect jaeger-build > /dev/null || docker buildx create --use --name=jaeger-build --buildkitd-flags="--allow-insecure-entitlement security.insecure --allow-insecure-entitlement network.host" --driver-opt="network=host"
	docker inspect registry > /dev/null || docker run --rm -d -p 5000:5000 --name registry registry:2

.PHONY: clean-docker-buildx
clean-docker-buildx:
	docker buildx rm jaeger-build
	docker rm -f registry
