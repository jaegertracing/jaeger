CREATE TABLE
    IF NOT EXISTS attribute_metadata (
        attribute_key String,
        type String  -- 'bool', 'double', 'int', 'string', 'bytes', 'map', 'slice'
    ) ENGINE = MergeTree
ORDER BY (attribute_key)
