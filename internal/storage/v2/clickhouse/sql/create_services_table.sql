CREATE TABLE
    IF NOT EXISTS services (name String) ENGINE = AggregatingMergeTree
ORDER BY
    (name);