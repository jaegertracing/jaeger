CREATE TABLE
    IF NOT EXISTS spans (
        id String CODEC(ZSTD(1)),
        trace_id String CODEC(ZSTD(1)),
        trace_state String CODEC(ZSTD(1)),
        parent_span_id String CODEC(ZSTD(1)),
        name LowCardinality(String) CODEC(ZSTD(1)),
        kind LowCardinality(String) CODEC(ZSTD(1)),
        start_time DateTime64 (9) CODEC(Delta(8), ZSTD(1)),
        status_code LowCardinality(String) CODEC(ZSTD(1)),
        status_message String CODEC(ZSTD(1)),
        duration Int64 CODEC(ZSTD(1)),
        bool_attributes Nested (key LowCardinality(String), value Bool) CODEC(ZSTD(1)),
        double_attributes Nested (key LowCardinality(String), value Float64) CODEC(ZSTD(1)),
        int_attributes Nested (key LowCardinality(String), value Int64) CODEC(ZSTD(1)),
        str_attributes Nested (key LowCardinality(String), value String) CODEC(ZSTD(1)),
        complex_attributes Nested (key LowCardinality(String), value String) CODEC(ZSTD(1)),
        events Nested (
            name LowCardinality(String),
            timestamp DateTime64 (9),
            bool_attributes Nested (key LowCardinality(String), value Bool),
            double_attributes Nested (key LowCardinality(String), value Float64),
            int_attributes Nested (key LowCardinality(String), value Int64),
            str_attributes Nested (key LowCardinality(String), value String),
            complex_attributes Nested (key LowCardinality(String), value String)
        ) CODEC(ZSTD(1)),
        links Nested (
            trace_id String,
            span_id String,
            trace_state String,
            bool_attributes Nested (key LowCardinality(String), value Bool),
            double_attributes Nested (key LowCardinality(String), value Float64),
            int_attributes Nested (key LowCardinality(String), value Int64),
            str_attributes Nested (key LowCardinality(String), value String),
            complex_attributes Nested (key LowCardinality(String), value String)
        ) CODEC(ZSTD(1)),
        service_name LowCardinality(String) CODEC(ZSTD(1)),
        resource_bool_attributes Nested (key LowCardinality(String), value Bool) CODEC(ZSTD(1)),
        resource_double_attributes Nested (key LowCardinality(String), value Float64) CODEC(ZSTD(1)),
        resource_int_attributes Nested (key LowCardinality(String), value Int64) CODEC(ZSTD(1)),
        resource_str_attributes Nested (key LowCardinality(String), value String) CODEC(ZSTD(1)),
        resource_complex_attributes Nested (key LowCardinality(String), value String) CODEC(ZSTD(1)),
        scope_name LowCardinality(String) CODEC(ZSTD(1)),
        scope_version LowCardinality(String) CODEC(ZSTD(1)),
        scope_bool_attributes Nested (key LowCardinality(String), value Bool) CODEC(ZSTD(1)),
        scope_double_attributes Nested (key LowCardinality(String), value Float64) CODEC(ZSTD(1)),
        scope_int_attributes Nested (key LowCardinality(String), value Int64) CODEC(ZSTD(1)),
        scope_str_attributes Nested (key LowCardinality(String), value String) CODEC(ZSTD(1)),
        scope_complex_attributes Nested (key LowCardinality(String), value String) CODEC(ZSTD(1)),
        INDEX idx_trace_id trace_id TYPE bloom_filter GRANULARITY 1,
        INDEX idx_duration duration TYPE minmax GRANULARITY 1
    ) ENGINE = {{ .MergeTree }}
PARTITION BY toDate(start_time)
ORDER BY (service_name, name, toDateTime(start_time))
{{- if gt .TTLSeconds 0 }}
TTL start_time + INTERVAL {{ .TTLSeconds }} SECOND DELETE
{{- end }}
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1
