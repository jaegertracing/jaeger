XDOCK_YAML=crossdock/docker-compose.yml

JAEGER_COMPOSE_YAML ?= crossdock/jaeger-docker-compose.yml
JAEGER_COLLECTOR_HC_PORT ?= 14269

.PHONY: crossdock
crossdock:
	docker-compose -f $(JAEGER_COMPOSE_YAML) -f $(XDOCK_YAML) kill
	docker-compose -f $(JAEGER_COMPOSE_YAML) -f $(XDOCK_YAML) rm -f test_driver
	JAEGER_COLLECTOR_HC_PORT=${JAEGER_COLLECTOR_HC_PORT} docker-compose -f $(JAEGER_COMPOSE_YAML) -f $(XDOCK_YAML) run crossdock 2>&1 | tee run-crossdock.log
	grep 'Tests passed!' run-crossdock.log

.PHONY: crossdock-otel
crossdock-otel:
	JAEGER_COMPOSE_YAML=crossdock/jaeger-opentelemetry-docker-compose.yml JAEGER_COLLECTOR_HC_PORT=13133 $(MAKE) crossdock

.PHONE: crossdock-logs
crossdock-logs:
	docker-compose -f $(JAEGER_COMPOSE_YAML) -f $(XDOCK_YAML) logs

.PHONE: crossdock-clean
crossdock-clean:
	docker-compose -f $(JAEGER_COMPOSE_YAML) -f $(XDOCK_YAML) down

