.PHONY: build
build: clean-jaeger
	cd ../../ && \
	make build-all-in-one && \
	make docker-images-jaeger-backend && \
	docker tag jaegertracing/all-in-one:latest jaegertracing/all-in-one:dev

.PHONY: run
run:
	docker compose -f docker-compose-connector.yml up

# This make target is setup to allow devs to run the older spanmetrics processor setup, for example,
# to test backwards compatibility of Jaeger with spanmetrics processor.
.PHONY: run-processor
run-processor:
	docker compose -f docker-compose-processor.yml up

.PHONY: clean-jaeger
clean-jaeger:
	# Also cleans up intermediate cached containers.
	docker system prune -f

.PHONY: clean-all
clean: clean-jaeger
	docker rmi -f jaegertracing/all-in-one:dev ; \
	docker rmi -f jaegertracing/all-in-one:latest ; \
	docker rmi -f otel/opentelemetry-collector-contrib:latest ; \
	docker rmi -f prom/prometheus:latest ; \
	docker rmi -f grafana/grafana:latest
