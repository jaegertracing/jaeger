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
        bool_attributes {{ .Layout.GroupType "Bool" }},
        double_attributes {{ .Layout.GroupType "Float64" }},
        int_attributes {{ .Layout.GroupType "Int64" }},
        str_attributes {{ .Layout.GroupType "String" }},
        complex_attributes {{ .Layout.GroupType "String" }},
        events Nested (
            name String,
            timestamp DateTime64 (9),
            bool_attributes {{ .Layout.GroupType "Bool" }},
            double_attributes {{ .Layout.GroupType "Float64" }},
            int_attributes {{ .Layout.GroupType "Int64" }},
            str_attributes {{ .Layout.GroupType "String" }},
            complex_attributes {{ .Layout.GroupType "String" }}
        ),
        links Nested (
            trace_id String,
            span_id String,
            trace_state String,
            bool_attributes {{ .Layout.GroupType "Bool" }},
            double_attributes {{ .Layout.GroupType "Float64" }},
            int_attributes {{ .Layout.GroupType "Int64" }},
            str_attributes {{ .Layout.GroupType "String" }},
            complex_attributes {{ .Layout.GroupType "String" }}
        ),
        service_name String,
        resource_bool_attributes {{ .Layout.GroupType "Bool" }},
        resource_double_attributes {{ .Layout.GroupType "Float64" }},
        resource_int_attributes {{ .Layout.GroupType "Int64" }},
        resource_str_attributes {{ .Layout.GroupType "String" }},
        resource_complex_attributes {{ .Layout.GroupType "String" }},
        scope_name String,
        scope_version String,
        scope_bool_attributes {{ .Layout.GroupType "Bool" }},
        scope_double_attributes {{ .Layout.GroupType "Float64" }},
        scope_int_attributes {{ .Layout.GroupType "Int64" }},
        scope_str_attributes {{ .Layout.GroupType "String" }},
        scope_complex_attributes {{ .Layout.GroupType "String" }},
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
