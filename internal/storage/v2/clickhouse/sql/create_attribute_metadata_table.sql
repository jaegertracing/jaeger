CREATE TABLE
    IF NOT EXISTS attribute_metadata{{ .Suffix }} (
        attribute_key String,
        type String,  -- 'bool', 'double', 'int', 'str', 'bytes', 'map', 'slice'
        level String  -- 'resource', 'scope', 'span'
    ) ENGINE = {{ .AggregatingMergeTree }}
ORDER BY (attribute_key, type, level)
