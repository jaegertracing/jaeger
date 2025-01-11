# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

BINARY ?= all-in-one # Default value uses v1 binary

.PHONY: build
build: clean-jaeger
	cd ../../ && make build-$(BINARY) GOOS=linux 
	cd ../../ && make create-baseimg PLATFORMS=linux/$(shell go env GOARCH)
	cd ../../ && docker buildx build --target release \
		--tag jaegertracing/$(BINARY):dev \
		--build-arg base_image=localhost:5000/baseimg_alpine:latest \
		--build-arg debug_image=not-used \
		--build-arg TARGETARCH=$(shell go env GOARCH) \
		--load \
		cmd/$(BINARY)

# starts up the system required for SPM using the latest otel image and a development jaeger image.
# Note: the jaeger "dev" image can be built with "make build".
.PHONY: dev
dev: export JAEGER_IMAGE_TAG = dev
dev: 
	docker compose -f docker-compose-v1.yml up $(DOCKER_COMPOSE_ARGS)

.PHONY: dev-v2
dev-v2: export JAEGER_IMAGE_TAG = dev
dev-v2: export BINARY = jaeger
dev-v2: build 
	docker compose -f docker-compose-v2.yml up $(DOCKER_COMPOSE_ARGS)

.PHONY: clean-jaeger
clean-jaeger:
	# Also cleans up intermediate cached containers.
	docker system prune -f

.PHONY: clean-all
clean-all: clean-jaeger
	docker rmi -f jaegertracing/all-in-one:dev ; \
	docker rmi -f jaegertracing/all-in-one:latest ; \
	docker rmi -f otel/opentelemetry-collector-contrib:latest ; \
	docker rmi -f prom/prometheus:latest ; \
