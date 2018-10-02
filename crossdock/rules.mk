XDOCK_YAML=crossdock/docker-compose.yml
JAEGER_STORAGE_BACKEND?=cassandra
JAEGER_COMPOSE_YAML=docker-compose/jaeger-docker-compose-$(JAEGER_STORAGE_BACKEND).yml

.PHONY: crossdock
crossdock: $(JAEGER_COMPOSE_YAML) $(XDOCK_YAML)
	docker-compose -f $(JAEGER_COMPOSE_YAML) -f $(XDOCK_YAML) kill
	docker-compose -f $(JAEGER_COMPOSE_YAML) -f $(XDOCK_YAML) rm -f test_driver
	docker-compose -f $(JAEGER_COMPOSE_YAML) up -d $(JAEGER_STORAGE_BACKEND)
	sleep 2
	docker-compose -f $(JAEGER_COMPOSE_YAML) -f $(XDOCK_YAML) run crossdock 2>&1 | tee run-crossdock.log
	grep 'Tests passed!' run-crossdock.log

.PHONE: crossdock-logs
crossdock-logs: $(JAEGER_COMPOSE_YAML) $(XDOCK_YAML)
	docker-compose -f $(JAEGER_COMPOSE_YAML) -f $(XDOCK_YAML) logs

.PHONE: crossdock-clean
crossdock-clean: $(JAEGER_COMPOSE_YAML) $(XDOCK_YAML)
	docker-compose -f $(JAEGER_COMPOSE_YAML) -f $(XDOCK_YAML) down
