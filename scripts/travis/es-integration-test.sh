#!/bin/bash

set -e

docker pull docker.elastic.co/elasticsearch/elasticsearch:5.6.12
CID=$(docker run -d -p 9200:9200 -e "http.host=0.0.0.0" -e "transport.host=127.0.0.1" -e "xpack.security.enabled=false" -e "xpack.monitoring.enabled=false" docker.elastic.co/elasticsearch/elasticsearch:5.6.12)

STORAGE=elasticsearch make storage-integration-test
make index-cleaner-integration-test

docker kill $CID

echo "Executing token propatagion test"

# Mock UI, needed only for build query service.
make build-crossdock-ui-placeholder
GOOS=linux make build-query

SPAN_STORAGE_TYPE=elasticsearch ./cmd/query/query-linux --es.server-urls=http://127.0.0.1:9200 --es.tls=false --query.bearer-token-propagation=true &

PID=$(echo $!)

make token-propagation-integration-test

kill -9 ${PID}