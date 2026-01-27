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
        INDEX idx_service_name service_name TYPE set(500) GRANULARITY 1,
        INDEX idx_name name TYPE set(1000) GRANULARITY 1,
        INDEX idx_start_time start_time TYPE minmax GRANULARITY 1,
        INDEX idx_duration duration TYPE minmax GRANULARITY 1,
        INDEX idx_attributes_keys str_attributes.key TYPE bloom_filter GRANULARITY 1,
        INDEX idx_attributes_values str_attributes.value TYPE bloom_filter GRANULARITY 1,
        INDEX idx_resource_attributes_keys resource_str_attributes.key TYPE bloom_filter GRANULARITY 1,
        INDEX idx_resource_attributes_values resource_str_attributes.value TYPE bloom_filter GRANULARITY 1,
    ) ENGINE = MergeTree
PARTITION BY toDate(start_time)
ORDER BY (trace_id)
{{- if .TTLSeconds }}
TTL start_time + INTERVAL {{ .TTLSeconds }} SECOND
{{- end }}