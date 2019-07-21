XDOCK_YAML=crossdock/docker-compose.yml

# FIXME temporarily switching back to tchannel
# See https://github.com/jaegertracing/jaeger/issues/1229
# JAEGER_COMPOSE_YAML=docker-compose/jaeger-docker-compose.yml
JAEGER_COMPOSE_YAML=crossdock/jaeger-docker-compose.yml

.PHONY: crossdock
crossdock:
	docker-compose -f $(JAEGER_COMPOSE_YAML) -f $(XDOCK_YAML) kill
	docker-compose -f $(JAEGER_COMPOSE_YAML) -f $(XDOCK_YAML) rm -f test_driver
	docker-compose -f $(JAEGER_COMPOSE_YAML) -f $(XDOCK_YAML) run crossdock 2>&1 | tee run-crossdock.log
	grep 'Tests passed!' run-crossdock.log

.PHONE: crossdock-logs
crossdock-logs:
	docker-compose -f $(JAEGER_COMPOSE_YAML) -f $(XDOCK_YAML) logs

.PHONE: crossdock-clean
crossdock-clean:
	docker-compose -f $(JAEGER_COMPOSE_YAML) -f $(XDOCK_YAML) down

