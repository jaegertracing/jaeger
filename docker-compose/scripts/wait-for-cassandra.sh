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

function wait_for_cassandra() {
	NC=$(command -v nc)
	if [[ -z "$NC" ]]; then
		NC=$(command -v netcat)
		if [[ -z "$NC" ]]; then
			echo "Cannot find nc/netcat" 1>&2
			exit 1
		fi
	fi

    CQLSH=${CQLSH:-"/usr/local/bin/cqlsh"}
	CQLSH_HOST=${CQLSH_HOST:-"cassandra"}
    CQL_VERSION=${CQL_VERSION:-"3.4.4"}
	CASSANDRA_WAIT_TIMEOUT=${CASSANDRA_WAIT_TIMEOUT:-"180"}
    KEYSPACE=${KEYSPACE:-"jaeger_v1_dc1"}

	total_wait=0

	while (( total_wait < CASSANDRA_WAIT_TIMEOUT ))
	do
		if  "${NC}" -z "${CQLSH_HOST}" 9042; then
            if (echo "DESCRIBE KEYSPACE ${KEYSPACE};" | "${CQLSH}" --cqlversion="${CQL_VERSION}" "${CQLSH_HOST}"); then
                break
            fi
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
