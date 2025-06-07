CREATE TABLE IF NOT EXISTS spans (
    id String,
    trace_id String,
    trace_state String,
    parent_span_id String,
    name String,
    kind String,
    start_time DateTime64(9),
    status_code String,
    status_message String,
    duration UInt64,

    events Nested (
        name String,
        timestamp DateTime64(9)
    ),

    
    links Nested (
        trace_id String,
        span_id String,
        trace_state String
    ),

    service_name String,

    scope_name String,
    scope_version String
)
ENGINE = MergeTree
PRIMARY KEY(trace_id);

CREATE TABLE IF NOT EXISTS services (
    name String
) ENGINE = AggregatingMergeTree
PRIMARY KEY(name);

CREATE MATERIALIZED VIEW IF NOT EXISTS services_mv
TO services
AS
SELECT service_name AS name
FROM spans
GROUP BY service_name;

CREATE TABLE IF NOT EXISTS operations (
    name String,
    span_kind String
) ENGINE = AggregatingMergeTree
PRIMARY KEY(name, span_kind);

CREATE MATERIALIZED VIEW IF NOT EXISTS operations_mv
TO operations
AS
SELECT
    name,
    kind AS span_kind
FROM spans
GROUP BY
    name,
    span_kind;