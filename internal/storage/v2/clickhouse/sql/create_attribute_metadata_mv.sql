CREATE MATERIALIZED VIEW IF NOT EXISTS attribute_metadata_mv TO attribute_metadata AS
SELECT
    tp.1 AS attribute_key,
    tp.2 AS type,
    tp.3 AS level
FROM (
    SELECT
        arrayJoin(arrayConcat(
            arrayMap(k -> (k, 'bool',   'span'),     {{ .KeysFn "bool_attributes" }}),
            arrayMap(k -> (k, 'bool',   'resource'), {{ .KeysFn "resource_bool_attributes" }}),
            arrayMap(k -> (k, 'bool',   'scope'),    {{ .KeysFn "scope_bool_attributes" }}),
            arrayMap(k -> (k, 'double', 'span'),     {{ .KeysFn "double_attributes" }}),
            arrayMap(k -> (k, 'double', 'resource'), {{ .KeysFn "resource_double_attributes" }}),
            arrayMap(k -> (k, 'double', 'scope'),    {{ .KeysFn "scope_double_attributes" }}),
            arrayMap(k -> (k, 'int',    'span'),     {{ .KeysFn "int_attributes" }}),
            arrayMap(k -> (k, 'int',    'resource'), {{ .KeysFn "resource_int_attributes" }}),
            arrayMap(k -> (k, 'int',    'scope'),    {{ .KeysFn "scope_int_attributes" }}),
            arrayMap(k -> (k, 'str',    'span'),     {{ .KeysFn "str_attributes" }}),
            arrayMap(k -> (k, 'str',    'resource'), {{ .KeysFn "resource_str_attributes" }}),
            arrayMap(k -> (k, 'str',    'scope'),    {{ .KeysFn "scope_str_attributes" }}),
            arrayMap(k -> (
                multiIf(startsWith(k, '@bytes@'), substring(k, 8),
                        startsWith(k, '@map@'),   substring(k, 6),
                        startsWith(k, '@slice@'), substring(k, 8), k),
                multiIf(startsWith(k, '@bytes@'), 'bytes',
                        startsWith(k, '@map@'),   'map',
                        startsWith(k, '@slice@'), 'slice', ''),
                'span'
            ), {{ .KeysFn "complex_attributes" }}),
            arrayMap(k -> (
                multiIf(startsWith(k, '@bytes@'), substring(k, 8),
                        startsWith(k, '@map@'),   substring(k, 6),
                        startsWith(k, '@slice@'), substring(k, 8), k),
                multiIf(startsWith(k, '@bytes@'), 'bytes',
                        startsWith(k, '@map@'),   'map',
                        startsWith(k, '@slice@'), 'slice', ''),
                'resource'
            ), {{ .KeysFn "resource_complex_attributes" }}),
            arrayMap(k -> (
                multiIf(startsWith(k, '@bytes@'), substring(k, 8),
                        startsWith(k, '@map@'),   substring(k, 6),
                        startsWith(k, '@slice@'), substring(k, 8), k),
                multiIf(startsWith(k, '@bytes@'), 'bytes',
                        startsWith(k, '@map@'),   'map',
                        startsWith(k, '@slice@'), 'slice', ''),
                'scope'
            ), {{ .KeysFn "scope_complex_attributes" }})
        )) AS tp
    FROM spans
)
GROUP BY attribute_key, type, level;
