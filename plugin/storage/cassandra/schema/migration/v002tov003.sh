#!/usr/bin/env bash

# Create a new operations table and copy all data from operation_names table
# Sample usage: KEYSPACE=jaeger_v1_test TIMEOUT=1000 ./plugin/storage/cassandra/schema/migration/v002tov003.sh

set -euo pipefail

function usage {
    >&2 echo "Error: $1"
    >&2 echo ""
    >&2 echo "Usage: KEYSPACE={keyspace} TTL={ttl} $0"
    >&2 echo ""
    >&2 echo "The following parameters can be set via environment:"
    >&2 echo "  KEYSPACE           - keyspace"
    >&2 echo ""
    exit 1
}

confirm() {
    read -r -p "${1:-Continue? [y/N]} " response
    case "$response" in
        [yY][eE][sS]|[yY])
            true
            ;;
        *)
            exit 1
            ;;
    esac
}

if [[ ${KEYSPACE} == "" ]]; then
   usage "missing KEYSPACE parameter"
fi

if [[ ${KEYSPACE} =~ [^a-zA-Z0-9_] ]]; then
    usage "invalid characters in KEYSPACE=$KEYSPACE parameter, please use letters, digits or underscores"
fi

keyspace=${KEYSPACE}
old_table=operation_names
new_table=operations
cqlsh_cmd=cqlsh

row_count=$(${cqlsh_cmd} -e "select count(*) from $keyspace.$old_table;"|head -4|tail -1| tr -d ' ')

echo "About to copy $row_count rows to new table..."

confirm

${cqlsh_cmd} -e "COPY $keyspace.$old_table (service_name, operation_name) to '$old_table.csv';"

if [[ ! -f ${old_table}.csv ]]; then
    echo "Could not find $old_table.csv. Backup from cassandra was probably not successful"
    exit 1
fi

csv_rows=$(wc -l ${old_table}.csv | tr -dc '0-9')

if [[ ${row_count} -ne ${csv_rows} ]]; then
    echo "Number of rows: $csv_rows in file is not equal to number of rows: $row_count in cassandra"
    exit 1
fi

echo "Generating data for new table..."
while IFS="," read service_name operation_name; do
    echo "$service_name,,$operation_name"
done < ${old_table}.csv > ${new_table}.csv

ttl=$(${cqlsh_cmd} -e "select default_time_to_live from system_schema.tables WHERE keyspace_name='$keyspace' AND table_name='$old_table';"|head -4|tail -1|tr -d ' ')

echo "Creating new table $new_table with ttl: $ttl"

${cqlsh_cmd} -e "CREATE TABLE IF NOT EXISTS $keyspace.$new_table (
    service_name        text,
    span_kind           text,
    operation_name      text,
    PRIMARY KEY ((service_name), span_kind, operation_name)
)
    WITH compaction = {
        'min_threshold': '4',
        'max_threshold': '32',
        'class': 'org.apache.cassandra.db.compaction.SizeTieredCompactionStrategy'
    }
    AND dclocal_read_repair_chance = 0.0
    AND default_time_to_live = $ttl
    AND speculative_retry = 'NONE'
    AND gc_grace_seconds = 10800;"

echo "Import data to new table: $keyspace.$new_table from $new_table.csv"

# empty string will be inserted as empty string instead of null
${cqlsh_cmd} -e "COPY $keyspace.$new_table (service_name, span_kind, operation_name)
    FROM '$new_table.csv'
    WITH NULL='NIL';"

echo "Data from old table are successfully imported to new table!"

echo "Before finish, do you want to delete old table: $keyspace.$old_table?"
confirm
${cqlsh_cmd} -e "DROP TABLE IF EXISTS $keyspace.$old_table;"