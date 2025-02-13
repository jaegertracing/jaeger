# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

BINARY ?= jaeger

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

.PHONY: dev
dev: export JAEGER_VERSION = dev
dev: build 
	docker compose -f docker-compose.yml up $(DOCKER_COMPOSE_ARGS)

.PHONY: clean-jaeger
clean-jaeger:
	# Also cleans up intermediate cached containers.
	docker system prune -f

.PHONY: clean-all
clean-all: clean-jaeger
	docker rmi -f otel/opentelemetry-collector-contrib:latest
