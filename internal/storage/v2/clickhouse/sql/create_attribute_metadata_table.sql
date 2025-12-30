CREATE TABLE
    IF NOT EXISTS attribute_metadata (
        attribute_key String,
        type String,  -- 'bool', 'double', 'int', 'str', 'bytes', 'map', 'slice'
        level String  -- 'resource', 'scope', 'span'
    ) ENGINE = ReplacingMergeTree
ORDER BY (attribute_key, type, level)
