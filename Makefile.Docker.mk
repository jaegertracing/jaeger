# Copyright (c) 2023 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# These DOCKER_xxx vars are used when building Docker images.
DOCKER_NAMESPACE?=jaegertracing
DOCKER_TAG?=latest

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
	GOOS=linux GOARCH=$(GOARCH) $(MAKE) build-es-rollover
	docker build -t $(DOCKER_NAMESPACE)/jaeger-es-index-cleaner:${DOCKER_TAG} --build-arg base_image=$(BASE_IMAGE) --build-arg TARGETARCH=$(GOARCH) cmd/es-index-cleaner
	docker build -t $(DOCKER_NAMESPACE)/jaeger-es-rollover:${DOCKER_TAG} --build-arg base_image=$(BASE_IMAGE) --build-arg TARGETARCH=$(GOARCH) cmd/es-rollover
	@echo "Finished building jaeger-elasticsearch tools =============="
