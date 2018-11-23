#!/bin/bash
#
# Based on https://stackoverflow.com/a/46037377/1930331.

function run_scripts() {
	until echo "DROP KEYSPACE IF EXISTS ${KEYSPACE};" | cqlsh 2>/dev/null; do
		echo "Waiting for Cassandra to initialize"
		sleep 2
	done

    DIR=$(cd $(dirname ${BASH_SOURCE[0]}) >/dev/null && pwd)
	"$DIR/create.sh" | cqlsh
	echo "Cassandra is available"
}

run_scripts &
exec /docker-entrypoint.sh "$@"
