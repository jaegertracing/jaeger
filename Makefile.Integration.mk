.PHONY: docker-images-jaeger-backend
docker-images-jaeger-backend: PLATFORMS=linux/amd64
docker-images-jaeger-backend: create-baseimg 
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
