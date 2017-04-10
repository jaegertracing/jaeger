XDOCK_YAML=crossdock/docker-compose.yml

QUERY_SRC = cmd/query/query-linux
QUERY_DST = crossdock/cmd/jaeger-query

AGENT_SRC = cmd/agent/agent-linux
AGENT_DST = crossdock/cmd/jaeger-agent

COLLECTOR_SRC = cmd/collector/collector-linux
COLLECTOR_DST = crossdock/cmd/jaeger-collector

CMD_DIR = crossdock/cmd

.PHONY: crossdock-copy-bin
crossdock-copy-bin:
	[ -d $(CMD_DIR) ] || mkdir -p $(CMD_DIR)
	cp $(QUERY_SRC) $(QUERY_DST)
	cp $(AGENT_SRC) $(AGENT_DST)
	cp $(COLLECTOR_SRC) $(COLLECTOR_DST)

.PHONY: crossdock
crossdock: $(SCHEMAS) crossdock-copy-bin
	#docker-compose -f $(XDOCK_YAML) kill test_driver go node java python
	docker-compose -f $(XDOCK_YAML) kill test_driver go
	docker-compose -f $(XDOCK_YAML) rm -f test_driver
	docker-compose -f $(XDOCK_YAML) build test_driver
	docker-compose -f $(XDOCK_YAML) run crossdock 2>&1 | tee run-crossdock.log
	grep 'Tests passed!' run-crossdock.log

.PHONY: crossdock-fresh
crossdock-fresh: $(SCHEMAS) crossdock-copy-bin
	docker-compose -f $(XDOCK_YAML) kill
	docker-compose -f $(XDOCK_YAML) rm --force
	docker-compose -f $(XDOCK_YAML) pull
	docker-compose -f $(XDOCK_YAML) build
	docker-compose -f $(XDOCK_YAML) run crossdock

.PHONE: crossdock-logs
crossdock-logs:
	docker-compose -f $(XDOCK_YAML) logs
