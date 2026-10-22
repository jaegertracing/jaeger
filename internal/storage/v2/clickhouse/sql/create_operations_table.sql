CREATE TABLE IF NOT EXISTS
    operations (
        service_name LowCardinality(String) CODEC(ZSTD(1)),
        name LowCardinality(String) CODEC(ZSTD(1)),
        span_kind LowCardinality(String) CODEC(ZSTD(1))
    ) ENGINE = {{ .AggregatingMergeTree }}
ORDER BY
    (service_name, span_kind);
