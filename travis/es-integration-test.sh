#!/bin/bash

docker pull docker.elastic.co/elasticsearch/elasticsearch:5.4.0
export CID=$(docker run -d -p 9200:9200 -e "http.host=0.0.0.0" -e "transport.host=127.0.0.1" docker.elastic.co/elasticsearch/elasticsearch:5.4.0)
export ES_INTEGRATION_TEST=test
make es-integration-test
unset ES_INTEGRATION_TEST
docker kill $CID