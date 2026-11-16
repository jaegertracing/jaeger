-- attribute metadata table
CREATE TABLE
    IF NOT EXISTS attribute_metadata_local (
        attribute_key String,
        type String,  -- 'bool', 'double', 'int', 'str', 'bytes', 'map', 'slice'
        level String  -- 'resource', 'scope', 'span'
    ) ENGINE = ReplicatedAggregatingMergeTree
ORDER BY (attribute_key, type, level)
