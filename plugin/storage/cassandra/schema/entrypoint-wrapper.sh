#!/bin/bash
#
# Based on https://stackoverflow.com/a/46037377/1930331.

function run_scripts() {
	until echo "DROP KEYSPACE IF EXISTS ${KEYSPACE};" | cqlsh localhost; do
		echo "waiting for cassandra to initialize: ${KEYSPACE}"
		sleep 2
	done

	/create.sh | cqlsh localhost
	echo "Cassandra is available"
}

run_scripts &
exec /docker-entrypoint.sh "$@"
