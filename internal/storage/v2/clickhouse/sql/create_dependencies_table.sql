CREATE TABLE
    IF NOT EXISTS dependencies (
        timestamp DateTime64 (9) CODEC(ZSTD(1)),
        dependencies_json String CODEC(ZSTD(1))
    ) ENGINE = {{ .MergeTree }}
PARTITION BY
    toDate(timestamp)
ORDER BY
    (timestamp);
