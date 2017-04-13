#!/bin/bash

# Notes:
# gc_grace_seconds is non-zero, see: http://www.uberobert.com/cassandra_gc_grace_disables_hinted_handoff/
# For TTL of 2 days, compaction window is 1 hour, rule of thumb here: http://thelastpickle.com/blog/2016/12/08/TWCS-part1.html
function usage {
    >&2 echo "Error: $1"
    >&2 echo "Usage: $0 {prod|test} [datacenter] | cqlsh"
    >&2 echo ""
    >&2 echo "Other parameters can be set via environment:"
    >&2 echo "  TTL - default time to live, in seconds (default: 172800, 2 days)"
    >&2 echo "  KEYSPACE - keyspace (default: jaeger_v1_$datacenter)"
    >&2 echo "  REPLICATION_FACTOR - replication factor for prod (default: 2)"
    exit 1
}

default_time_to_live=${TTL:-172800}
replication_factor=${REPLICATION_FACTOR:-2}

if [[ "$1" == "" ]]; then
    usage "missing environment"
elif [[ "$1" == "prod" ]]; then
    if [[ "$2" == "" ]]; then usage "missing datacenter"; fi
    datacenter=$2
    replication="{'class': 'NetworkTopologyStrategy', '$datacenter': '${replication_factor}' }"
elif [[ "$1" == "test" ]]; then 
    datacenter=${2:-'test'}
    replication="{'class': 'SimpleStrategy', 'replication_factor': '1'}"
else
    usage "invalid environment $1, expecting prod or test"
fi

keyspace=${KEYSPACE:-"jaeger_v1_${datacenter}"}

cat - <<EOF
CREATE KEYSPACE IF NOT EXISTS ${keyspace} WITH replication = ${replication};

CREATE TYPE IF NOT EXISTS ${keyspace}.keyvalue (
    key             text,
    value_type      text,
    value_string    text,
    value_bool      boolean,
    value_long      bigint,
    value_double    double,
    value_binary    blob,
);

CREATE TYPE IF NOT EXISTS ${keyspace}.log (
    ts      bigint,
    fields  list<frozen<keyvalue>>,
);

CREATE TYPE IF NOT EXISTS ${keyspace}.span_ref (
    ref_type        text,
    trace_id        blob,
    span_id         bigint,
);

CREATE TYPE IF NOT EXISTS ${keyspace}.process (
    service_name    text,
    tags            list<frozen<keyvalue>>,
);

 -- Notice we have span_hash. This exists only for zipkin backwards compat. Zipkin allows spans with the same ID.
 -- Note: Cassandra re-orders non-PK columns alphabetically, so the table looks differently in CQLSH "describe table".
 -- start_time is bigint instead of timestamp as we require microsecond precision
CREATE TABLE IF NOT EXISTS ${keyspace}.traces (
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
    PRIMARY KEY (trace_id, span_id, span_hash)
)
    WITH compaction = {'compaction_window_size': '1', 'compaction_window_unit': 'HOURS', 'class': 'org.apache.cassandra.db.compaction.TimeWindowCompactionStrategy'}
    AND dclocal_read_repair_chance = 0.0
    AND default_time_to_live = ${default_time_to_live}
    AND speculative_retry = 'NONE'
    AND gc_grace_seconds = 10800; -- 3 hours of downtime acceptable on nodes

CREATE TABLE IF NOT EXISTS ${keyspace}.service_names (
    service_name text,
    PRIMARY KEY (service_name)
) WITH compaction = {'min_threshold': '4', 'class': 'org.apache.cassandra.db.compaction.SizeTieredCompactionStrategy', 'max_threshold': '32'}
    AND dclocal_read_repair_chance = 0.0
    AND default_time_to_live = ${default_time_to_live}
    AND speculative_retry = 'NONE'
    AND gc_grace_seconds = 10800; -- 3 hours of downtime acceptable on nodes

CREATE TABLE IF NOT EXISTS ${keyspace}.operation_names (
    service_name        text,
    operation_name      text,
    PRIMARY KEY ((service_name), operation_name)
) WITH compaction = {'min_threshold': '4', 'class': 'org.apache.cassandra.db.compaction.SizeTieredCompactionStrategy', 'max_threshold': '32'}
    AND dclocal_read_repair_chance = 0.0
    AND default_time_to_live = ${default_time_to_live}
    AND speculative_retry = 'NONE'
    AND gc_grace_seconds = 10800; -- 3 hours of downtime acceptable on nodes

-- we need indices on:
-- start_time (this is inherent in all indices)
-- duration
-- service_name
-- service and operation_name
-- tags

-- index of trace IDs by service + operation names, sorted by span start_time.
CREATE TABLE IF NOT EXISTS ${keyspace}.service_operation_index (
    service_name        text,
    operation_name      text,
    start_time          bigint,
    trace_id            blob,
    PRIMARY KEY ((service_name, operation_name), start_time)
) WITH CLUSTERING ORDER BY (start_time DESC)
    AND compaction = {'compaction_window_size': '1', 'compaction_window_unit': 'HOURS', 'class': 'org.apache.cassandra.db.compaction.TimeWindowCompactionStrategy'}
    AND dclocal_read_repair_chance = 0.0
    AND default_time_to_live = ${default_time_to_live}
    AND speculative_retry = 'NONE'
    AND gc_grace_seconds = 10800; -- 3 hours of downtime acceptable on nodes

CREATE TABLE IF NOT EXISTS ${keyspace}.service_name_index (
    service_name      text,
    bucket            int,
    start_time        bigint,
    trace_id          blob,
    PRIMARY KEY ((service_name, bucket), start_time)
) WITH CLUSTERING ORDER BY (start_time DESC)
    AND compaction = {'compaction_window_size': '1', 'compaction_window_unit': 'HOURS', 'class': 'org.apache.cassandra.db.compaction.TimeWindowCompactionStrategy'}
    AND dclocal_read_repair_chance = 0.0
    AND default_time_to_live = ${default_time_to_live}
    AND speculative_retry = 'NONE'
    AND gc_grace_seconds = 10800; -- 3 hours of downtime acceptable on nodes

CREATE TABLE IF NOT EXISTS ${keyspace}.duration_index (
    service_name    text,      // service name
    operation_name  text,      // operation name, or blank for queries without span name
    bucket          timestamp, // time bucket, - the start_time of the given span rounded to an hour
    duration        bigint,    // span duration, in microseconds
    start_time      bigint,
    trace_id        blob,
    PRIMARY KEY ((service_name, operation_name, bucket), duration, start_time, trace_id)
) WITH CLUSTERING ORDER BY (duration DESC, start_time DESC)
    AND compaction = {'compaction_window_size': '1', 'compaction_window_unit': 'HOURS', 'class': 'org.apache.cassandra.db.compaction.TimeWindowCompactionStrategy'}
    AND dclocal_read_repair_chance = 0.0
    AND default_time_to_live = ${default_time_to_live}
    AND speculative_retry = 'NONE'
    AND gc_grace_seconds = 10800; -- 3 hours of downtime acceptable on nodes

-- this will go away once Cassanrda supports indexing of nested collections: https://issues.apache.org/jira/browse/CASSANDRA-11182
-- a bucketing strategy may have to be added for tag queries
-- we can make this table even better by adding a timestamp to it
CREATE TABLE IF NOT EXISTS ${keyspace}.tag_index (
    service_name    text,
    tag_key         text,
    tag_value       text,
    start_time      bigint,
    trace_id        blob,
    span_id         bigint,
    PRIMARY KEY ((service_name, tag_key, tag_value), start_time, trace_id, span_id)
)
    WITH CLUSTERING ORDER BY (start_time DESC)
    AND compaction = {'compaction_window_size': '1', 'compaction_window_unit': 'HOURS', 'class': 'org.apache.cassandra.db.compaction.TimeWindowCompactionStrategy'}
    AND dclocal_read_repair_chance = 0.0
    AND default_time_to_live = ${default_time_to_live}
    AND speculative_retry = 'NONE'
    AND gc_grace_seconds = 10800; -- 3 hours of downtime acceptable on nodes

CREATE TYPE IF NOT EXISTS ${keyspace}.dependency (
    parent          text,
    child           text,
    call_count      bigint,
);

-- compaction strategy is intentionally different as compared to other tables due to the size of dependencies data
-- note we have to write ts twice (once as ts_index). This is because we cannot make a SASI index on the primary key
CREATE TABLE IF NOT EXISTS ${keyspace}.dependencies (
    ts          timestamp,
    ts_index    timestamp,
    dependencies list<frozen<dependency>>,
    PRIMARY KEY (ts)
) WITH compaction = {'min_threshold': '4', 'class': 'org.apache.cassandra.db.compaction.SizeTieredCompactionStrategy', 'max_threshold': '32'};

CREATE CUSTOM INDEX ON ${keyspace}.dependencies (ts_index) USING 'org.apache.cassandra.index.sasi.SASIIndex' WITH OPTIONS = {'mode': 'SPARSE'};
EOF
