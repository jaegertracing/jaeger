CREATE TABLE
    IF NOT EXISTS services (name LowCardinality(String) CODEC(ZSTD(1))) ENGINE = {{ .AggregatingMergeTree }}
ORDER BY
    (name);
