#!/bin/bash
#
# This script is used in the Docker image jaegertracing/jaeger-cassandra-schema
# that allows installing Jaeger keyspace and schema without installing cqlsh.

CQLSH=${CQLSH:-"/usr/bin/cqlsh"}
CQLSH_HOST=${CQLSH_HOST:-"cassandra"}
CASSANDRA_WAIT_TIMEOUT=${CASSANDRA_WAIT_TIMEOUT:-"60"}
DATACENTER=${DATACENTER:-"dc1"}
KEYSPACE=${KEYSPACE:-"jaeger_v1_${DATACENTER}"}
MODE=${MODE:-"test"}

total_wait=0
while true
do
  ${CQLSH} -e "describe keyspaces"
  if (( $? == 0 )); then
    break
  else
    if (( total_wait >= ${CASSANDRA_WAIT_TIMEOUT} )); then
      echo "Timed out waiting for Cassandra."
      exit 1
    fi
    echo "Cassandra is still not up at ${CQLSH_HOST}. Waiting 1 second."
    sleep 1s
    ((total_wait++))
  fi
done

echo "Generating the schema for the keyspace ${KEYSPACE} and datacenter ${DATACENTER}"

set -ex

MODE="${MODE}" DATACENTER="${DATACENTER}" KEYSPACE="${KEYSPACE}" /cassandra-schema/create.sh | ${CQLSH}

echo "Schema created" | tee /cassandra-schema/ready.txt
