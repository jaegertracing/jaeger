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
	CQLSH=${CQLSH:-"/usr/bin/cqlsh"}
	CQLSH_HOST=${CQLSH_HOST:-"cassandra"}
	CQLSH_SSL=${CQLSH_SSL:-""}
	CASSANDRA_WAIT_TIMEOUT=${CASSANDRA_WAIT_TIMEOUT:-"180"}

	total_wait=0

	while (( total_wait < CASSANDRA_WAIT_TIMEOUT ))
	do
		if nc -z "${CQLSH_HOST}" 9042; then
			break
		fi
		echo "Cassandra is still not up at ${CQLSH_HOST}. Waiting 1 second."
		sleep 1s
		((total_wait++))
	done

	while (( total_wait < CASSANDRA_WAIT_TIMEOUT ))
	do
	  if ${CQLSH} "${CQLSH_SSL}" -e "describe keyspaces"; then
		break
		fi
		echo "Cassandra is still not up at ${CQLSH_HOST}. Waiting 1 second."
		sleep 1s
		((total_wait++))
	done

	if (( total_wait >= CASSANDRA_WAIT_TIMEOUT )); then
		echo "Timed out waiting for Cassandra."
		exit 1
	fi

	exec $@
}

wait_for_cassandra $@
