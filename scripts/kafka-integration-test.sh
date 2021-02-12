#!/bin/bash

set -e

docker pull spotify/kafka
CID=$(docker run -d -p 2181:2181 -p 9092:9092 --env ADVERTISED_HOST=localhost --env ADVERTISED_PORT=9092 --rm spotify/kafka)

# Guarantees no matter what happens, docker will remove the instance at the end.
trap 'docker rm -f $CID 2>/dev/null' EXIT INT TERM

export STORAGE=kafka
while true; do
    if nc -z localhost 9092; then
        break
    fi
done
make storage-integration-test
