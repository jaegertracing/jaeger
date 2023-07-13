.PHONY: build
build: export DOCKER_TAG = dev
build: clean-jaeger
	cd ../../ && \
	make build-all-in-one && \
	make docker-images-jaeger-backend

# run starts up the system required for SPM using the latest jaeger and otel images.
.PHONY: run
run: export JAEGER_IMAGE_TAG = latest
run: _run-connector

# run starts up the system required for SPM using the latest otel image and a development jaeger image.
# Note: the jaeger "dev" image can be built with "make build".
.PHONY: run-dev
run-dev: export JAEGER_IMAGE_TAG = dev
run-dev: _run-connector

# _run-connector is the base target to bring up the system required for SPM using the new OTEL spanmetrics connector.
.PHONY: _run-connector
_run-connector: export OTEL_IMAGE_TAG = 0.80.0
_run-connector: export OTEL_CONFIG_SRC = ./otel-collector-config-connector.yml
_run-connector: export PROMETHEUS_QUERY_SUPPORT_SPANMETRICS_CONNECTOR = true
_run-connector:
	docker compose -f docker-compose.yml up

# run the older spanmetrics processor setup, for example,
# to test backwards compatibility of Jaeger with spanmetrics processor.
.PHONY: run-dev-processor
run-dev-processor: export JAEGER_IMAGE_TAG = dev
# Fix to a version before the breaking changes were introduced.
run-dev-processor: export OTEL_IMAGE_TAG = 0.70.0
run-dev-processor: export OTEL_CONFIG_SRC = ./otel-collector-config-processor.yml
run-dev-processor:
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
