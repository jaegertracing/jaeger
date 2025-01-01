# Copyright (c) 2023 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

DOCKER_NAMESPACE ?= jaegertracing
DOCKER_TAG       ?= latest
DOCKER_REGISTRY  ?= localhost:5000
BASE_IMAGE       ?= $(DOCKER_REGISTRY)/baseimg_alpine:latest
DEBUG_IMAGE      ?= $(DOCKER_REGISTRY)/debugimg_alpine:latest

create-baseimg-debugimg: create-baseimg create-debugimg

create-baseimg: prepare-docker-buildx
	@echo "::group:: create-baseimg"
	docker buildx build -t $(BASE_IMAGE) --push \
		--platform=$(LINUX_PLATFORMS) \
		docker/base
	@echo "::endgroup::"

create-debugimg: prepare-docker-buildx
	@echo "::group:: create-debugimg"
	docker buildx build -t $(DEBUG_IMAGE) --push \
		--platform=$(LINUX_PLATFORMS) \
		docker/debug
	@echo "::endgroup::"

create-fake-debugimg: prepare-docker-buildx
	@echo "::group:: create-fake-debugimg"
	docker buildx build -t $(DEBUG_IMAGE) --push \
		--platform=$(LINUX_PLATFORMS) \
		docker/base
	@echo "::endgroup::"

.PHONY: prepare-docker-buildx
prepare-docker-buildx:
	@echo "::group:: prepare-docker-buildx"
	docker buildx inspect jaeger-build > /dev/null || docker buildx create --use --name=jaeger-build --buildkitd-flags="--allow-insecure-entitlement security.insecure --allow-insecure-entitlement network.host" --driver-opt="network=host"
	docker inspect registry > /dev/null || docker run --rm -d -p 5000:5000 --name registry registry:2
	@echo "::endgroup::"

.PHONY: clean-docker-buildx
clean-docker-buildx:
	docker buildx rm jaeger-build
	docker rm -f registry

.PHONY: docker-hotrod
docker-hotrod:
	GOOS=linux $(MAKE) build-examples
	docker build -t $(DOCKER_NAMESPACE)/example-hotrod:${DOCKER_TAG} ./examples/hotrod --build-arg TARGETARCH=$(GOARCH)
	@echo "Finished building hotrod =============="

.PHONY: docker-images-tracegen
docker-images-tracegen:
	docker build -t $(DOCKER_NAMESPACE)/jaeger-tracegen:${DOCKER_TAG} cmd/tracegen/ --build-arg TARGETARCH=$(GOARCH)
	@echo "Finished building jaeger-tracegen =============="

.PHONY: docker-images-anonymizer
docker-images-anonymizer:
	docker build -t $(DOCKER_NAMESPACE)/jaeger-anonymizer:${DOCKER_TAG} cmd/anonymizer/ --build-arg TARGETARCH=$(GOARCH)
	@echo "Finished building jaeger-anonymizer =============="

.PHONY: docker-images-cassandra
docker-images-cassandra:
	docker build -t $(DOCKER_NAMESPACE)/jaeger-cassandra-schema:${DOCKER_TAG} plugin/storage/cassandra/
	@echo "Finished building jaeger-cassandra-schema =============="

.PHONY: docker-images-elastic
docker-images-elastic: create-baseimg
	GOOS=linux GOARCH=$(GOARCH) $(MAKE) build-esmapping-generator
	GOOS=linux GOARCH=$(GOARCH) $(MAKE) build-es-index-cleaner
	docker build -t $(DOCKER_NAMESPACE)/jaeger-es-index-cleaner:${DOCKER_TAG} --build-arg base_image=$(BASE_IMAGE) --build-arg TARGETARCH=$(GOARCH) cmd/es-index-cleaner
	@echo "Finished building jaeger-elasticsearch tools =============="