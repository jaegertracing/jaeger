CREATE MATERIALIZED VIEW IF NOT EXISTS attribute_metadata_mv TO attribute_metadata AS
SELECT
    tp.1 AS attribute_key,
    tp.2 AS type,
    tp.3 AS level
FROM (
    SELECT
        arrayJoin(arrayConcat(
            arrayMap(k -> (k, 'bool',   'span'),     bool_attributes.key),
            arrayMap(k -> (k, 'bool',   'resource'), resource_bool_attributes.key),
            arrayMap(k -> (k, 'bool',   'scope'),    scope_bool_attributes.key),
            arrayMap(k -> (k, 'double', 'span'),     double_attributes.key),
            arrayMap(k -> (k, 'double', 'resource'), resource_double_attributes.key),
            arrayMap(k -> (k, 'double', 'scope'),    scope_double_attributes.key),
            arrayMap(k -> (k, 'int',    'span'),     int_attributes.key),
            arrayMap(k -> (k, 'int',    'resource'), resource_int_attributes.key),
            arrayMap(k -> (k, 'int',    'scope'),    scope_int_attributes.key),
            arrayMap(k -> (k, 'str',    'span'),     str_attributes.key),
            arrayMap(k -> (k, 'str',    'resource'), resource_str_attributes.key),
            arrayMap(k -> (k, 'str',    'scope'),    scope_str_attributes.key),
            arrayMap(k -> (
                if(startsWith(k, '@bytes@'), substring(k, 8), if(startsWith(k, '@map@'), substring(k, 6), if(startsWith(k, '@slice@'), substring(k, 8), k))),
                if(startsWith(k, '@bytes@'), 'bytes', if(startsWith(k, '@map@'), 'map', if(startsWith(k, '@slice@'), 'slice', ''))),
                'span'
            ), complex_attributes.key),
            arrayMap(k -> (
                if(startsWith(k, '@bytes@'), substring(k, 8), if(startsWith(k, '@map@'), substring(k, 6), if(startsWith(k, '@slice@'), substring(k, 8), k))),
                if(startsWith(k, '@bytes@'), 'bytes', if(startsWith(k, '@map@'), 'map', if(startsWith(k, '@slice@'), 'slice', ''))),
                'resource'
            ), resource_complex_attributes.key),
            arrayMap(k -> (
                if(startsWith(k, '@bytes@'), substring(k, 8), if(startsWith(k, '@map@'), substring(k, 6), if(startsWith(k, '@slice@'), substring(k, 8), k))),
                if(startsWith(k, '@bytes@'), 'bytes', if(startsWith(k, '@map@'), 'map', if(startsWith(k, '@slice@'), 'slice', ''))),
                'scope'
            ), scope_complex_attributes.key)
        )) AS tp
    FROM spans
)
GROUP BY attribute_key, type, level;