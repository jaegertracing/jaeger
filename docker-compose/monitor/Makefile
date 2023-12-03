.PHONY: build
build: export DOCKER_TAG = dev
build: clean-jaeger
	cd ../../ && \
	make build-all-in-one-linux && \
	make docker-images-jaeger-backend

# starts up the system required for SPM using the latest otel image and a development jaeger image.
# Note: the jaeger "dev" image can be built with "make build".
.PHONY: dev
dev: export JAEGER_IMAGE_TAG = dev
dev:
	docker compose -f docker-compose.yml up

# starts older spanmetrics processor setup, for example,
# to test backwards compatibility of Jaeger with spanmetrics processor.
.PHONY: dev-processor
dev-processor: export JAEGER_IMAGE_TAG = dev
# Fix to a version before the breaking changes were introduced.
dev-processor: export OTEL_IMAGE_TAG = 0.70.0
dev-processor: export OTEL_CONFIG_SRC = ./otel-collector-config-processor.yml
dev-processor: export PROMETHEUS_QUERY_SUPPORT_SPANMETRICS_CONNECTOR = false
dev-processor:
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
