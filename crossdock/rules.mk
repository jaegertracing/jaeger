XDOCK_YAML=crossdock/docker-compose.yml

SCHEMA = crossdock/scripts/schema.cql
SCHEMA_SRC = plugin/storage/cassandra/cassandra3v001-schema.sh

QUERY_SRC = cmd/query/query-linux
QUERY_DST = crossdock/cmd/jaeger-query

AGENT_SRC = cmd/agent/agent-linux
AGENT_DST = crossdock/cmd/jaeger-agent

COLLECTOR_SRC = cmd/collector/collector-linux
COLLECTOR_DST = crossdock/cmd/jaeger-collector

SCRIPTS_DIR = crossdock/scripts
CMD_DIR = crossdock/cmd

$(SCHEMA): $(SCHEMA_SRC)
	[ -d $(SCRIPTS_DIR) ] || mkdir -p $(SCRIPTS_DIR)
	# Remove all comments and multiple new lines and replace keyspace with jaeger from cql file
	$(SCHEMA_SRC) test | sed -E 's/ ?--.*//g' | sed 's/jaeger_v1_test/jaeger/g' | cat -s > $(SCHEMA)

.PHONY: crossdock-copy-bin
crossdock-copy-bin:
	[ -d $(CMD_DIR) ] || mkdir -p $(CMD_DIR)
	cp $(QUERY_SRC) $(QUERY_DST)
	cp $(AGENT_SRC) $(AGENT_DST)
	cp $(COLLECTOR_SRC) $(COLLECTOR_DST)

.PHONY: crossdock
crossdock: $(SCHEMA) crossdock-copy-bin
	#docker-compose -f $(XDOCK_YAML) kill test_driver go node java python
	docker-compose -f $(XDOCK_YAML) kill test_driver go
	docker-compose -f $(XDOCK_YAML) rm -f test_driver
	docker-compose -f $(XDOCK_YAML) build test_driver
	docker-compose -f $(XDOCK_YAML) run crossdock 2>&1 | tee run-crossdock.log
	grep 'Tests passed!' run-crossdock.log

.PHONY: crossdock-fresh
crossdock-fresh: $(SCHEMA) crossdock-copy-bin
	docker-compose -f $(XDOCK_YAML) kill
	docker-compose -f $(XDOCK_YAML) rm --force
	docker-compose -f $(XDOCK_YAML) pull
	docker-compose -f $(XDOCK_YAML) build
	docker-compose -f $(XDOCK_YAML) run crossdock

.PHONE: crossdock-logs
crossdock-logs:
	docker-compose -f $(XDOCK_YAML) logs
