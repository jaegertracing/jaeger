#!/bin/bash

set -e

run_integration_test() {
  docker_tag=$1
  docker pull docker.elastic.co/elasticsearch/elasticsearch:${docker_tag}
  CID=$(docker run --rm -d -p 9200:9200 -e "http.host=0.0.0.0" -e "transport.host=127.0.0.1" -e "xpack.security.enabled=false" -e "xpack.monitoring.enabled=false" docker.elastic.co/elasticsearch/elasticsearch:${docker_tag})
  STORAGE=elasticsearch make storage-integration-test
  make index-cleaner-integration-test
  docker kill $CID
}

run_integration_test "5.6.16"
run_integration_test "6.8.2"
ES_VERSION=7 run_integration_test "6.8.2"
run_integration_test "7.3.0"

echo "Executing token propatagion test"

# Mock UI, needed only for build query service.
make build-crossdock-ui-placeholder
GOOS=linux make build-query

SPAN_STORAGE_TYPE=elasticsearch ./cmd/query/query-linux --es.server-urls=http://127.0.0.1:9200 --es.tls=false --es.version=7 --query.bearer-token-propagation=true &
PID=$(echo $!)
make token-propagation-integration-test
kill -9 ${PID}
