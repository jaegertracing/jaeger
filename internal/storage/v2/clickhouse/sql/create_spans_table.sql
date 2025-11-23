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
        bool_attributes Nested (key String, value Bool),
        double_attributes Nested (key String, value Float64),
        int_attributes Nested (key String, value Int64),
        str_attributes Nested (key String, value String),
        complex_attributes Nested (key String, value String),
        events Nested (
            name String,
            timestamp DateTime64 (9),
            bool_attributes Nested (key String, value Bool),
            double_attributes Nested (key String, value Float64),
            int_attributes Nested (key String, value Int64),
            str_attributes Nested (key String, value String),
            complex_attributes Nested (key String, value String)
        ),
        links Nested (
            trace_id String,
            span_id String,
            trace_state String,
            bool_attributes Nested (key String, value Bool),
            double_attributes Nested (key String, value Float64),
            int_attributes Nested (key String, value Int64),
            str_attributes Nested (key String, value String),
            complex_attributes Nested (key String, value String)
        ),
        service_name String,
        resource_bool_attributes Nested (key String, value Bool),
        resource_double_attributes Nested (key String, value Float64),
        resource_int_attributes Nested (key String, value Int64),
        resource_str_attributes Nested (key String, value String),
        resource_complex_attributes Nested (key String, value String),
        scope_name String,
        scope_version String,
        scope_bool_attributes Nested (key String, value Bool),
        scope_double_attributes Nested (key String, value Float64),
        scope_int_attributes Nested (key String, value Int64),
        scope_str_attributes Nested (key String, value String),
        scope_complex_attributes Nested (key String, value String),
        INDEX idx_span_name name TYPE set(0) GRANULARITY 1,
        INDEX idx_service_name service_name TYPE set(0) GRANULARITY 1
    ) ENGINE = MergeTree
ORDER BY
    (trace_id)