#!/usr/bin/env bash

# This is the follow up to v002tov003part2.sh. It truncates and deletes the old traces table.

set -euo pipefail

function usage {
    >&2 echo "Error: $1"
    >&2 echo ""
    >&2 echo "Usage: KEYSPACE={keyspace} $0"
    >&2 echo ""
    >&2 echo "The following parameters can be set via environment:"
    >&2 echo "  KEYSPACE           - keyspace"
    >&2 echo "  TIMEOUT            - cqlsh request timeout"
    >&2 echo ""
    exit 1
}

confirm() {
    read -r -p "${1:-Are you sure? [y/N]} " response
    case "$response" in
        [yY][eE][sS]|[yY])
            true
            ;;
        *)
            exit 1
            ;;
    esac
}

keyspace=${KEYSPACE}
timeout=${TIMEOUT}
cqlsh_cmd=cqlsh --request-timeout=$timeout

if [[ ${keyspace} == "" ]]; then
   usage "missing KEYSPACE parameter"
fi

if [[ ${keyspace} =~ [^a-zA-Z0-9_] ]]; then
    usage "invalid characters in KEYSPACE=$keyspace parameter, please use letters, digits or underscores"
fi


row_count=$($cqlsh_cmd -e "select count(*) from $keyspace.traces;"|head -4|tail -1| tr -d ' ')

echo "About to delete $row_count rows."
confirm

$cqlsh_cmd -e "DROP TABLE IF EXISTS $keyspace.traces;"
