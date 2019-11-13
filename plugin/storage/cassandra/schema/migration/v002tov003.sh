#!/usr/bin/env bash

# update the operation_names table to add span_kind column

set -euo pipefail

function usage {
    >&2 echo "Error: $1"
    >&2 echo ""
    >&2 echo "Usage: KEYSPACE={keyspace} $0"
    >&2 echo ""
    >&2 echo "The following parameters can be set via environment:"
    >&2 echo "  KEYSPACE           - keyspace"
    >&2 echo ""
    exit 1
}

keyspace=${KEYSPACE}

cqlsh_cmd=cqlsh

if [[ ${keyspace} == "" ]]; then
   usage "missing KEYSPACE parameter"
fi

if [[ ${keyspace} =~ [^a-zA-Z0-9_] ]]; then
    usage "invalid characters in KEYSPACE=$keyspace parameter, please use letters, digits or underscores"
fi

$cqlsh_cmd -e "ALTER TABLE $keyspace.operation_names ADD span_kind text;"
echo "Added column 'span_kind' to $keyspace.operation_names"

KEYSPACE=jaeger_v1_test TIMEOUT=10 ./plugin/storage/cassandra/schema/migration/v002tov003.sh | $CQLSH_CMD