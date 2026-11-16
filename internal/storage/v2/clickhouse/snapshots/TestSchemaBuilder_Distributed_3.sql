-- services table
CREATE TABLE
    IF NOT EXISTS services_local (name String) ENGINE = ReplicatedAggregatingMergeTree
ORDER BY
    (name);
