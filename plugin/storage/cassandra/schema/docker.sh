#!/usr/bin/env bash
#
# This script is used in the Docker image jaegertracing/jaeger-cassandra-schema
# that allows installing Jaeger keyspace and schema without installing cqlsh.

CQLSH=${CQLSH:-"/opt/cassandra/bin/cqlsh"}
CQLSH_HOST=${CQLSH_HOST:-"cassandra"}
CQLSH_PORT=${CQLSH_PORT:-"9042"}
CQLSH_SSL=${CQLSH_SSL:-""}
CASSANDRA_WAIT_TIMEOUT=${CASSANDRA_WAIT_TIMEOUT:-"60"}
DATACENTER=${DATACENTER:-"dc1"}
KEYSPACE=${KEYSPACE:-"jaeger_v1_${DATACENTER}"}
MODE=${MODE:-"test"}
TEMPLATE=${TEMPLATE:-""}
USER=${CASSANDRA_USERNAME:-""}
PASSWORD=${CASSANDRA_PASSWORD:-""}
SCHEMA_SCRIPT=${SCHEMA_SCRIPT:-"/cassandra-schema/create.sh"}

if [ -z "$PASSWORD" ]; then
  CQLSH_CMD="${CQLSH} ${CQLSH_SSL} ${CQLSH_HOST} ${CQLSH_PORT}"
else
  CQLSH_CMD="${CQLSH} ${CQLSH_SSL} ${CQLSH_HOST} ${CQLSH_PORT} -u ${USER} -p ${PASSWORD}"
fi

total_wait=0
while true
do
  ${CQLSH_CMD} -e "describe keyspaces"
  if (( $? == 0 )); then
    break
  else
    if (( total_wait >= ${CASSANDRA_WAIT_TIMEOUT} )); then
      echo "Timed out waiting for Cassandra."
      exit 1
    fi
    echo "Cassandra is still not up at ${CQLSH_HOST}:${CQLSH_PORT}. Waiting 1 second."
    sleep 1s
    ((total_wait++))
  fi
done

# Extract cassandra version
#
# $ cqlsh -e "show version"
# [cqlsh 5.0.1 | Cassandra 3.11.11 | CQL spec 3.4.4 | Native protocol v4]
VERSION=
if [ -z "$TEMPLATE" ]; then
  VERSION=$(${CQLSH_CMD} -e "show version" \
      | awk -F "|" '{print $2}' \
      | awk -F " " '{print $2}' \
      | awk -F "." '{print $1}' \
  )
  echo "Cassandra version detected: ${VERSION}"
fi

echo "Generating the schema for the keyspace ${KEYSPACE} and datacenter ${DATACENTER}."

MODE="${MODE}" DATACENTER="${DATACENTER}" KEYSPACE="${KEYSPACE}" VERSION="${VERSION}" ${SCHEMA_SCRIPT} "${TEMPLATE}" | ${CQLSH_CMD}
