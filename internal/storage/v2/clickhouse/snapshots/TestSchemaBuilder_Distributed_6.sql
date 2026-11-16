-- operations table
CREATE TABLE IF NOT EXISTS
    operations_local (
        service_name String,
        name String,
        span_kind String
    ) ENGINE = ReplicatedAggregatingMergeTree
ORDER BY
    (service_name, span_kind);
