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
        scope_name String,
        scope_version String
    ) ENGINE = MergeTree PRIMARY KEY (trace_id)