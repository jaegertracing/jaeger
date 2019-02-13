#!/usr/bin/env bash

set -euo pipefail

function usage {
    >&2 echo "Error: $1"
    >&2 echo ""
    >&2 echo "Usage: KEYSPACE={keyspace} $0"
    >&2 echo ""
    >&2 echo "The following parameters can be set via environment:"
    >&2 echo "  KEYSPACE           - keyspace "
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

if [[ ${keyspace} == "" ]]; then
   usage "missing KEYSPACE parameter"
fi

if [[ ${keyspace} =~ [^a-zA-Z0-9_] ]]; then
    usage "invalid characters in KEYSPACE=$keyspace parameter, please use letters, digits or underscores"
fi

row_count=$(cqlsh -e "select count(*) from $keyspace.dependencies;"|head -4|tail -1| tr -d ' ')

echo "About to copy $row_count rows."
confirm

cqlsh -e "COPY $keyspace.dependencies (ts, dependencies ) to 'dependencies.csv';"

if [ ! -f dependencies.csv ]; then
    echo "Could not find dependencies.csv. Backup from cassandra was probably not successful"
    exit 1
fi

if [ ${row_count} -ne $(wc -l dependencies.csv | cut -f 1 -d ' ') ]; then
    echo "Number of rows in file is not equal to number of rows in cassandra"
    exit 1
fi

while IFS="," read ts dependency; do
    bucket=`date +"%Y-%m-%d%z" -d "$ts"`
    echo "$bucket,$ts,$dependency"
done < dependencies.csv > dependencies_datebucket.csv

dependencies_ttl=$(cqlsh -e "select default_time_to_live from system_schema.tables WHERE keyspace_name='$keyspace' AND table_name='dependencies';"|head -4|tail -1|tr -d ' ')

echo "Setting dependencies_ttl to $dependencies_ttl"

cqlsh -e "ALTER TYPE $keyspace.dependency ADD source text;"

cqlsh -e "CREATE TABLE $keyspace.dependencies_v2 (
    ts_bucket    timestamp,
    ts           timestamp,
    dependencies list<frozen<dependency>>,
    PRIMARY KEY (ts_bucket, ts)
) WITH CLUSTERING ORDER BY (ts DESC)
    AND compaction = {
        'min_threshold': '4',
        'max_threshold': '32',
        'class': 'org.apache.cassandra.db.compaction.SizeTieredCompactionStrategy'
    }
    AND default_time_to_live = $dependencies_ttl;
"

cqlsh -e "COPY $keyspace.dependencies_v2 (ts_bucket, ts, dependencies) FROM 'dependencies_datebucket.csv';"
