# Copyright (c) 2023 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

include crossdock/rules.mk

.PHONY: build-crossdock-binary
build-crossdock-binary:
	$(GOBUILD) -o ./crossdock/crossdock-$(GOOS)-$(GOARCH) ./crossdock/main.go

.PHONY: build-crossdock-linux
build-crossdock-linux:
	GOOS=linux $(MAKE) build-crossdock-binary

# Crossdock tests do not require fully functioning UI, so we skip it to speed up the build.
.PHONY: build-crossdock-ui-placeholder
build-crossdock-ui-placeholder:
	mkdir -p jaeger-ui/packages/jaeger-ui/build/
	cp cmd/query/app/ui/placeholder/index.html jaeger-ui/packages/jaeger-ui/build/index.html
	$(MAKE) build-ui

.PHONY: build-crossdock
build-crossdock: build-crossdock-ui-placeholder build-binaries-linux build-crossdock-linux docker-images-cassandra crossdock-docker-images-jaeger-backend
	docker build -t $(DOCKER_NAMESPACE)/test-driver:${DOCKER_TAG} --build-arg TARGETARCH=$(GOARCH) crossdock/
	@echo "Finished building test-driver ==============" ; \

.PHONY: build-and-run-crossdock
build-and-run-crossdock: build-crossdock
	make crossdock

.PHONY: build-crossdock-fresh
build-crossdock-fresh: build-crossdock-linux
	make crossdock-fresh

.PHONY: crossdock-docker-images-jaeger-backend
crossdock-docker-images-jaeger-backend: create-baseimg create-debugimg
	for component in "jaeger-agent" "jaeger-collector" "jaeger-query" "jaeger-ingester" "all-in-one" ; do \
		regex="jaeger-(.*)"; \
		component_suffix=$$component; \
		if [[ $$component =~ $$regex ]]; then \
			component_suffix="$${BASH_REMATCH[1]}"; \
		fi; \
		docker buildx build --target $(TARGET) \
			--tag $(DOCKER_NAMESPACE)/$$component$(SUFFIX):${DOCKER_TAG} \
			--build-arg base_image=$(BASE_IMAGE) \
			--build-arg debug_image=$(DEBUG_IMAGE) \
			--build-arg TARGETARCH=$(GOARCH) \
			--load \
			cmd/$$component_suffix; \
		echo "Finished building $$component ==============" ; \
	done;
