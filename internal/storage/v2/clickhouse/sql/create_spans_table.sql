CREATE TABLE
    IF NOT EXISTS spans{{ .LocalTableSuffix }} (
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
        bool_attributes {{ .ColumnTypeFn "bool" }},
        double_attributes {{ .ColumnTypeFn "double" }},
        int_attributes {{ .ColumnTypeFn "int" }},
        str_attributes {{ .ColumnTypeFn "str" }},
        complex_attributes {{ .ColumnTypeFn "complex" }},
        events Nested (
            name String,
            timestamp DateTime64 (9),
            bool_attributes {{ .ColumnTypeFn "bool" }},
            double_attributes {{ .ColumnTypeFn "double" }},
            int_attributes {{ .ColumnTypeFn "int" }},
            str_attributes {{ .ColumnTypeFn "str" }},
            complex_attributes {{ .ColumnTypeFn "complex" }}
        ),
        links Nested (
            trace_id String,
            span_id String,
            trace_state String,
            bool_attributes {{ .ColumnTypeFn "bool" }},
            double_attributes {{ .ColumnTypeFn "double" }},
            int_attributes {{ .ColumnTypeFn "int" }},
            str_attributes {{ .ColumnTypeFn "str" }},
            complex_attributes {{ .ColumnTypeFn "complex" }}
        ),
        service_name String,
        resource_bool_attributes {{ .ColumnTypeFn "bool" }},
        resource_double_attributes {{ .ColumnTypeFn "double" }},
        resource_int_attributes {{ .ColumnTypeFn "int" }},
        resource_str_attributes {{ .ColumnTypeFn "str" }},
        resource_complex_attributes {{ .ColumnTypeFn "complex" }},
        scope_name String,
        scope_version String,
        scope_bool_attributes {{ .ColumnTypeFn "bool" }},
        scope_double_attributes {{ .ColumnTypeFn "double" }},
        scope_int_attributes {{ .ColumnTypeFn "int" }},
        scope_str_attributes {{ .ColumnTypeFn "str" }},
        scope_complex_attributes {{ .ColumnTypeFn "complex" }},
{{- range .HoistedColumns }}
        {{ . }},
{{- end }}
        INDEX idx_trace_id trace_id TYPE bloom_filter GRANULARITY 1,
        INDEX idx_duration duration TYPE minmax GRANULARITY 1
    ) ENGINE = {{ .MergeTree }}
PARTITION BY toDate(start_time)
ORDER BY (service_name, name, toDateTime(start_time))
{{- if gt .TTLSeconds 0 }}
TTL start_time + INTERVAL {{ .TTLSeconds }} SECOND DELETE
{{- end }}
