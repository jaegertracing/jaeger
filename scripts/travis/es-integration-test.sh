#!/bin/bash

set -e

docker pull docker.elastic.co/elasticsearch/elasticsearch:5.6.12
CID=$(docker run -d -p 9200:9200 -e "http.host=0.0.0.0" -e "transport.host=127.0.0.1" -e "xpack.security.enabled=false" -e "xpack.monitoring.enabled=false" docker.elastic.co/elasticsearch/elasticsearch:5.6.12)

STORAGE=elasticsearch make storage-integration-test
make index-cleaner-integration-test

docker kill $CID
