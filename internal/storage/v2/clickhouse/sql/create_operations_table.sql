CREATE TABLE IF NOT EXISTS
    operations (
        service_name String,
        name String,
        span_kind String
    ) ENGINE = AggregatingMergeTree
ORDER BY
    (service_name, span_kind);