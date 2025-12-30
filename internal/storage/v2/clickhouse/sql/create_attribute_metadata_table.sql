CREATE TABLE
    IF NOT EXISTS attribute_metadata (
        attribute_key String,
        type String  -- 'bool', 'double', 'int', 'string', 'bytes', 'map', 'slice'
    ) ENGINE = ReplacingMergeTree
ORDER BY (attribute_key, type)
