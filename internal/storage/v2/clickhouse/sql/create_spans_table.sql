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
        INDEX idx_trace_id trace_id TYPE bloom_filter GRANULARITY 1,
        INDEX idx_duration duration TYPE minmax GRANULARITY 1,
<<<<<<< HEAD
        INDEX idx_span_str_attr_keys str_attributes.key TYPE bloom_filter(0.01) GRANULARITY 4,
        INDEX idx_span_str_attr_values str_attributes.value TYPE bloom_filter(0.01) GRANULARITY 4,
        INDEX idx_res_str_attr_keys resource_str_attributes.key TYPE bloom_filter(0.01) GRANULARITY 4,
        INDEX idx_res_str_attr_values resource_str_attributes.value TYPE bloom_filter(0.01) GRANULARITY 4,
        INDEX idx_scope_str_attr_keys scope_str_attributes.key TYPE bloom_filter(0.01) GRANULARITY 4,
        INDEX idx_scope_str_attr_values scope_str_attributes.value TYPE bloom_filter(0.01) GRANULARITY 4
=======
        INDEX idx_span_str_attr_keys str_attributes.key TYPE bloom_filter(0.01) GRANULARITY 1,
        INDEX idx_span_str_attr_values str_attributes.value TYPE bloom_filter(0.01) GRANULARITY 1,
        INDEX idx_res_str_attr_keys resource_str_attributes.key TYPE bloom_filter(0.01) GRANULARITY 1,
        INDEX idx_res_str_attr_values resource_str_attributes.value TYPE bloom_filter(0.01) GRANULARITY 1,
        INDEX idx_scope_str_attr_keys scope_str_attributes.key TYPE bloom_filter(0.01) GRANULARITY 1,
        INDEX idx_scope_str_attr_values scope_str_attributes.value TYPE bloom_filter(0.01) GRANULARITY 1
>>>>>>> e0ebabf1 (feat(clickhouse): Add bloom filter skip indexes for attribute search performance)
    ) ENGINE = MergeTree
PARTITION BY toDate(start_time)
ORDER BY (service_name, name, toDateTime(start_time))
{{- if gt .TTLSeconds 0 }}
TTL start_time + INTERVAL {{ .TTLSeconds }} SECOND DELETE
{{- end }}
