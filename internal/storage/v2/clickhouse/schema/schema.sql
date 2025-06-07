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
ORDER BY (start_time, trace_id, id);
