#!/bin/bash

set -ex

# Build the schema container and run it rather than using the existing container in Docker Hub since that
# requires this current build to succeed before this test can use it; chicken and egg problem.
docker build -t jaeger-cassandra-schema-integration-test plugin/storage/cassandra/
docker run -e CQLSH_HOST=localhost -e CQLSH_PORT=9042 -e TEMPLATE=/cassandra-schema/$1.cql.tmpl --network=host jaeger-cassandra-schema-integration-test
docker run -e CQLSH_HOST=localhost -e CQLSH_PORT=9043 -e TEMPLATE=/cassandra-schema/$2.cql.tmpl --network=host jaeger-cassandra-schema-integration-test
docker run -e CQLSH_HOST=localhost -e CQLSH_PORT=9044 -e TEMPLATE=/cassandra-schema/$3.cql.tmpl --network=host jaeger-cassandra-schema-integration-test

# Run the test.
export STORAGE=cassandra
make storage-integration-test
