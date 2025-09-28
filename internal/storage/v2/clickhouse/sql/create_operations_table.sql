CREATE TABLE
    IF NOT EXISTS operations (
        name String,
        span_kind String,
        service_name String
    ) ENGINE = AggregatingMergeTree PRIMARY KEY (name, span_kind)