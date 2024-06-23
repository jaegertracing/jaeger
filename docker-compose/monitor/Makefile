.PHONY: build
build: clean-jaeger
	cd ../../ && make build-all-in-one-linux
	cd ../../ && make create-baseimg PLATFORMS=linux/$(shell go env GOARCH)
	cd ../../ && docker buildx build --target release \
		--tag jaegertracing/all-in-one:dev \
		--build-arg base_image=localhost:5000/baseimg_alpine:latest \
		--build-arg debug_image=not-used \
		--build-arg TARGETARCH=$(shell go env GOARCH) \
		--load \
		cmd/all-in-one

# starts up the system required for SPM using the latest otel image and a development jaeger image.
# Note: the jaeger "dev" image can be built with "make build".
.PHONY: dev
dev: export JAEGER_IMAGE_TAG = dev
dev:
	docker compose -f docker-compose.yml up

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
	docker rmi -f grafana/grafana:latest
