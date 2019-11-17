#!/usr/bin/env bash

# Corresponds to migration changes in v003.cql.tmpl (addition of warnings field to the traces table). This script backs up
# all the existing data, creates the traces_v2 table and dumps all the data into traces_v2. This should be followed by
# v002tov003part2.sh which truncates the old traces table and deletes it.

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
timeout=${TIMEOUT:-"60"}
cqlsh_cmd="cqlsh --request-timeout=$timeout"

if [[ ${keyspace} == "" ]]; then
   usage "missing KEYSPACE parameter"
fi

if [[ ${keyspace} =~ [^a-zA-Z0-9_] ]]; then
    usage "invalid characters in KEYSPACE=$keyspace parameter, please use letters, digits or underscores"
fi

row_count=$($cqlsh_cmd -e "select count(*) from $keyspace.traces;"|head -4|tail -1| tr -d ' ')

echo "About to copy $row_count rows."
confirm

$cqlsh_cmd -e "COPY $keyspace.traces to 'existing_traces.csv';"

if [ ! -f existing_traces.csv ]; then
    echo "Could not find existing_traces.csv. Backup of existing traces from cassandra was probably not successful"
    exit 1
fi

if [ ${row_count} -ne $(wc -l existing_traces.csv | cut -f 1 -d ' ') ]; then
    echo "Number of rows in file is not equal to number of rows in cassandra"
    exit 1
fi

traces_ttl=$($cqlsh_cmd -e "select default_time_to_live from system_schema.tables WHERE keyspace_name='$keyspace' AND table_name='traces';" | head -4 | tail -1 | tr -d ' ')

echo "Setting traces_ttl to $traces_ttl"

$cqlsh_cmd -e "ALTER TYPE $keyspace.traces ADD warnings list<frozen<text>>;"

$cqlsh_cmd -e "CREATE TABLE IF NOT EXISTS ${keyspace}.traces_v2 (
    trace_id        blob,
    span_id         bigint,
    span_hash       bigint,
    parent_id       bigint,
    operation_name  text,
    flags           int,
    start_time      bigint,
    duration        bigint,
    tags            list<frozen<keyvalue>>,
    logs            list<frozen<log>>,
    refs            list<frozen<span_ref>>,
    process         frozen<process>,
    warnings        list<text>,
    PRIMARY KEY (trace_id, span_id, span_hash)
)
    WITH compaction = {
        'compaction_window_size': '1',
        'compaction_window_unit': 'HOURS',
        'class': 'org.apache.cassandra.db.compaction.TimeWindowCompactionStrategy'
    }
    AND dclocal_read_repair_chance = 0.0
    AND default_time_to_live = $traces_ttl
    AND speculative_retry = 'NONE'
    AND gc_grace_seconds = 10800;
"

$cqlsh_cmd -e "COPY $keyspace.traces_v2 (trace_id, span_id, span_hash, parent_id, operation_name, flags, start_time, duration, tags, logs, refs, process) FROM 'existing_traces.csv';"
