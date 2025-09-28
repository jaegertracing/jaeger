CREATE TABLE
    IF NOT EXISTS operations (name String, span_kind String) ENGINE = AggregatingMergeTree PRIMARY KEY (name, span_kind)