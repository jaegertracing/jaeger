#!/usr/bin/env bash
#
# This script is used in the Docker image jaegertracing/jaeger-cassandra-schema
# that allows installing Jaeger keyspace and schema without installing cqlsh.

CQLSH=${CQLSH:-"/opt/cassandra/bin/cqlsh"}
CQLSH_HOST=${CQLSH_HOST:-"cassandra"}
CQLSH_SSL=${CQLSH_SSL:-""}
CASSANDRA_WAIT_TIMEOUT=${CASSANDRA_WAIT_TIMEOUT:-"60"}
DATACENTER=${DATACENTER:-"dc1"}
KEYSPACE=${KEYSPACE:-"jaeger_v1_${DATACENTER}"}
MODE=${MODE:-"test"}
TEMPLATE=${TEMPLATE:-""}
USER=${CASSANDRA_USERNAME:-""}
PASSWORD=${CASSANDRA_PASSWORD:-""}

total_wait=0
while true
do
  if [ -z "$PASSWORD" ]; then
    ${CQLSH} ${CQLSH_SSL} ${CQLSH_HOST} -e "describe keyspaces"
  else
    ${CQLSH} ${CQLSH_SSL} ${CQLSH_HOST} -u ${USER} -p ${PASSWORD} -e "describe keyspaces"
  fi
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


if [ -z "$PASSWORD" ]; then
  MODE="${MODE}" DATACENTER="${DATACENTER}" KEYSPACE="${KEYSPACE}" /cassandra-schema/create.sh "${TEMPLATE}" | ${CQLSH} ${CQLSH_SSL} ${CQLSH_HOST}
else
  MODE="${MODE}" DATACENTER="${DATACENTER}" KEYSPACE="${KEYSPACE}" /cassandra-schema/create.sh "${TEMPLATE}" | ${CQLSH} ${CQLSH_SSL} ${CQLSH_HOST} -u ${USER} -p ${PASSWORD}
fi
