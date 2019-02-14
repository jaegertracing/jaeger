#!/bin/bash

set -e

# Clean up before starting.
docker rm -f cassandra || true
docker rm -f cassandra2 || true
docker network rm integration_test || true

# Create a network so that the schema container can communicate with the cassandra containers.
docker network create integration_test

# Start cassandra containers whose ports are exposed to localhost to facilitate testing.
docker run -d --name cassandra --network integration_test -p 9042:9042 -p 9160:9160 cassandra:3.9
docker run -d --name cassandra2 --network integration_test -p 9043:9042 -p 9161:9160 cassandra:3.9

# Build the schema container and run it rather than using the existing container in Docker Hub since that
# requires this current build to succeed before this test can use it; chicken and egg problem.
docker build -t jaeger-cassandra-schema-integration-test plugin/storage/cassandra/
docker run --network integration_test -e CQLSH_HOST=cassandra -e TEMPLATE=/cassandra-schema/v001.cql.tmpl jaeger-cassandra-schema-integration-test
docker run --network integration_test -e CQLSH_HOST=cassandra2 -e TEMPLATE=/cassandra-schema/v002.cql.tmpl jaeger-cassandra-schema-integration-test

# Run the test.
export STORAGE=cassandra
make storage-integration-test

# Tear down after.
docker rm -f cassandra
docker rm -f cassandra2
docker network rm integration_test
