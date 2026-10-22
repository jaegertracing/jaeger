CREATE TABLE
    IF NOT EXISTS attribute_metadata (
        attribute_key LowCardinality(String) CODEC(ZSTD(1)),
        type LowCardinality(String) CODEC(ZSTD(1)),  -- 'bool', 'double', 'int', 'str', 'bytes', 'map', 'slice'
        level LowCardinality(String) CODEC(ZSTD(1))  -- 'resource', 'scope', 'span'
    ) ENGINE = {{ .AggregatingMergeTree }}
ORDER BY (attribute_key, type, level)
