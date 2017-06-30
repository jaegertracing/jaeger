#!/bin/bash

set -e

docker pull docker.elastic.co/elasticsearch/elasticsearch:5.4.0
export CID=$(docker run -d -p 9200:9200 -e "http.host=0.0.0.0" -e "transport.host=127.0.0.1" docker.elastic.co/elasticsearch/elasticsearch:5.4.0)
export ESINTEGRATIONTEST=test
make es-integration-test
export ESINTEGRATIONTEST=
docker kill $CID