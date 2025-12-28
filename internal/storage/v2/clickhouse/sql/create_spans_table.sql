CREATE TABLE
    IF NOT EXISTS spans (
        id String,
        trace_id String,
        trace_state String,
        parent_span_id String,
        name String,
        kind String,
        start_time DateTime64 (9),
        status_code String,
        status_message String,
        duration Int64,
        attributes Nested (key String, value String, type String),
        events Nested (
            name String,
            timestamp DateTime64 (9),
            attributes Nested (key String, value String, type String)
        ),
        links Nested (
            trace_id String,
            span_id String,
            trace_state String,
            attributes Nested (key String, value String, type String)
        ),
        service_name String,
        resource_attributes Nested (key String, value String, type String),
        scope_name String,
        scope_version String,
        scope_attributes Nested (key String, value String, type String),
        INDEX idx_service_name service_name TYPE set(500) GRANULARITY 1,
        INDEX idx_name name TYPE set(1000) GRANULARITY 1,
        INDEX idx_start_time start_time TYPE minmax GRANULARITY 1,
        INDEX idx_duration duration TYPE minmax GRANULARITY 1,
        INDEX idx_attributes_keys attributes.key TYPE bloom_filter GRANULARITY 1,
        INDEX idx_attributes_values attributes.value TYPE bloom_filter GRANULARITY 1,
        INDEX idx_resource_attributes_keys resource_attributes.key TYPE bloom_filter GRANULARITY 1,
        INDEX idx_resource_attributes_values resource_attributes.value TYPE bloom_filter GRANULARITY 1,
    ) ENGINE = MergeTree
PARTITION BY toDate(start_time)
ORDER BY (trace_id)