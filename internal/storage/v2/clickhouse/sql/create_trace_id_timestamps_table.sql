CREATE TABLE IF NOT EXISTS trace_id_timestamps
(
    trace_id String,
    start DateTime64(9),
    end DateTime64(9)
)
ENGINE = MergeTree()
ORDER BY (trace_id)
{{- if gt .TTLSeconds 0 }}
TTL start + INTERVAL {{ .TTLSeconds }} SECOND DELETE
{{- end }}