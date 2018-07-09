#!/bin/bash

set -e

docker pull spotify/kafka
CID=$(docker run -d -p 2181:2181 -p 9092:9092 --env ADVERTISED_HOST=localhost --env ADVERTISED_PORT=9092 --rm spotify/kafka)
export STORAGE=kafka
make storage-integration-test
docker kill $CID
