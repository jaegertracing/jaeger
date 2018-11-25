#!/bin/bash
#
# This script can be used to enforce dependency on Cassandra initialization. The
# arguments are not invoked until Cassandra is available.
#
# Example:
#
# $ wait-for-cassandra.sh echo test
# ...   # wait for cassandra initialization
# test  # `echo test` is invoked

CQLSH=${CQLSH:-"/cassandra/bin/cqlsh"}
CQLSH_HOST=${CQLSH_HOST:-"cassandra"}
CQLSH_SSL=${CQLSH_SSL:-""}
CQLSH_PROTOCOL=${CQLSH_PROTOCOL:-"--cqlversion=3.4.2"}
CASSANDRA_WAIT_TIMEOUT=${CASSANDRA_WAIT_TIMEOUT:-"180"}
DATACENTER=${DATACENTER:-"dc1"}
KEYSPACE=${KEYSPACE:-"jaeger_v1_${DATACENTER}"}
MODE=${MODE:-"test"}
TABLES=${TABLES:-"traces,tag_index,service_names,operation_names"}


function wait_for_cassandra() {
    query="describe keyspace ${KEYSPACE};"
    tables_array=(${TABLES//,/ })
    for t in "${tables_array[@]}"; do
        query+="describe table ${KEYSPACE}.${t};"
    done

    NC=$(command -v nc)
    if [[ -z "$NC" ]]; then
        NC=$(command -v netcat)
        if [[ -z "$NC" ]]; then
            echo "Cannot find nc/netcat" 1>&2
            exit 1
        fi
    fi

    total_wait=0

    while (( total_wait < CASSANDRA_WAIT_TIMEOUT ))
    do
        if "${NC}" -z "${CQLSH_HOST}" 9042 && \
           ${CQLSH} ${CQLSH_SSH} ${CQLSH_PROTOCOL} ${CQLSH_HOST} -e "$query" > /dev/null
        then
            # Give Cassandra another 5 seconds to make tables available.
            sleep 5
            break
        fi
        echo "Cassandra is still not up at ${CQLSH_HOST}. Waiting 1 second ($total_wait/$CASSANDRA_WAIT_TIMEOUT)."
        sleep 1s
        ((total_wait++))
    done

    if (( total_wait >= CASSANDRA_WAIT_TIMEOUT )); then
        echo "Timed out waiting for Cassandra."
        exit 1
    fi
}

wait_for_cassandra
exec $@
