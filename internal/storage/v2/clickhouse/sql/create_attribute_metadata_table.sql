CREATE TABLE
    IF NOT EXISTS attribute_metadata (
        attribute_key String,
        storage_type String  -- 'bool', 'double', 'int', 'string', 'bytes', 'map', 'slice'
    ) ENGINE = MergeTree
ORDER BY (attribute_key)
