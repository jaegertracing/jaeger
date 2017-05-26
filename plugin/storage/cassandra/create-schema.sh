#!/bin/bash

CQLSH_HOST=${CQLSH_HOST:-"cassandra"}
CASSANDRA_WAIT_TIMEOUT=${CASSANDRA_WAIT_TIMEOUT:-"60"}
DATACENTER=${DATACENTER:-"dc1"}
KEYSPACE=${KEYSPACE:-"jaeger_v1_${DATACENTER}"}

total_wait=0
while true
do
  /opt/apache-cassandra-3.0.12/bin/cqlsh -e "describe keyspaces" > /dev/null 2>&1
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
export KEYSPACE

# the `test` parameter is to force the script to use a SimpleStrategy instead of
# NetworkTopologyStrategy .
/cassandra-schema/cassandra3v001-schema.sh test "${DATACENTER}" | \
  /opt/apache-cassandra-3.0.12/bin/cqlsh
