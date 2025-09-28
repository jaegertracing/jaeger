CREATE TABLE
    IF NOT EXISTS services (name String) ENGINE = AggregatingMergeTree PRIMARY KEY (name)