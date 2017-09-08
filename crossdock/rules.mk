XDOCK_YAML=crossdock/docker-compose.yml

BUILD_DIR = crossdock/.build
CMD_DIR = $(BUILD_DIR)/cmd
SCRIPTS_DIR = $(BUILD_DIR)/scripts

SCHEMA = $(SCRIPTS_DIR)/schema.cql
SCHEMA_SRC = plugin/storage/cassandra/schema/create.sh

QUERY_SRC = cmd/query/query-linux
QUERY_DST = $(CMD_DIR)/jaeger-query

AGENT_SRC = cmd/agent/agent-linux
AGENT_DST = $(CMD_DIR)/jaeger-agent

COLLECTOR_SRC = cmd/collector/collector-linux
COLLECTOR_DST = $(CMD_DIR)/jaeger-collector

$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

$(CMD_DIR): $(BUILD_DIR)
	mkdir -p $(CMD_DIR)

$(SCRIPTS_DIR): $(BUILD_DIR)
	mkdir -p $(SCRIPTS_DIR)

$(SCHEMA): $(SCRIPTS_DIR) $(SCHEMA_SRC)
	MODE=test KEYSPACE=jaeger $(SCHEMA_SRC) | cat -s > $(SCHEMA)

.PHONY: crossdock-copy-bin
crossdock-copy-bin: $(CMD_DIR)
	cp $(QUERY_SRC) $(QUERY_DST)
	cp $(AGENT_SRC) $(AGENT_DST)
	cp $(COLLECTOR_SRC) $(COLLECTOR_DST)

.PHONY: crossdock
crossdock: $(SCHEMA) crossdock-copy-bin
	docker-compose -f $(XDOCK_YAML) kill test_driver go node java python
	docker-compose -f $(XDOCK_YAML) rm -f test_driver
	docker-compose -f $(XDOCK_YAML) build test_driver
	docker-compose -f $(XDOCK_YAML) run crossdock 2>&1 | tee run-crossdock.log
	grep 'Tests passed!' run-crossdock.log

.PHONY: crossdock-fresh
crossdock-fresh: $(SCHEMA) crossdock-copy-bin
	docker-compose -f $(XDOCK_YAML) down --rmi all
	docker-compose -f $(XDOCK_YAML) run crossdock

.PHONE: crossdock-logs
crossdock-logs:
	docker-compose -f $(XDOCK_YAML) logs
