CREATE TABLE
    operations (
        service_name String,
        name String,
        span_kind String
    ) ENGINE = ReplacingMergeTree
ORDER BY
    (service_name, name, span_kind);