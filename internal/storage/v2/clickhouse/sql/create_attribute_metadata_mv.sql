CREATE MATERIALIZED VIEW IF NOT EXISTS attribute_metadata_mv TO attribute_metadata AS
SELECT
    tp.1 AS attribute_key,
    tp.2 AS type,
    tp.3 AS level
FROM (
    SELECT
        arrayJoin(arrayConcat(
            arrayMap(k -> (k, 'bool',   'span'),     {{ .Layout.Keys "bool_attributes" }}),
            arrayMap(k -> (k, 'bool',   'resource'), {{ .Layout.Keys "resource_bool_attributes" }}),
            arrayMap(k -> (k, 'bool',   'scope'),    {{ .Layout.Keys "scope_bool_attributes" }}),
            arrayMap(k -> (k, 'double', 'span'),     {{ .Layout.Keys "double_attributes" }}),
            arrayMap(k -> (k, 'double', 'resource'), {{ .Layout.Keys "resource_double_attributes" }}),
            arrayMap(k -> (k, 'double', 'scope'),    {{ .Layout.Keys "scope_double_attributes" }}),
            arrayMap(k -> (k, 'int',    'span'),     {{ .Layout.Keys "int_attributes" }}),
            arrayMap(k -> (k, 'int',    'resource'), {{ .Layout.Keys "resource_int_attributes" }}),
            arrayMap(k -> (k, 'int',    'scope'),    {{ .Layout.Keys "scope_int_attributes" }}),
            arrayMap(k -> (k, 'str',    'span'),     {{ .Layout.Keys "str_attributes" }}),
            arrayMap(k -> (k, 'str',    'resource'), {{ .Layout.Keys "resource_str_attributes" }}),
            arrayMap(k -> (k, 'str',    'scope'),    {{ .Layout.Keys "scope_str_attributes" }}),
            arrayMap(k -> (
                multiIf(startsWith(k, '@bytes@'), substring(k, 8),
                        startsWith(k, '@map@'),   substring(k, 6),
                        startsWith(k, '@slice@'), substring(k, 8), k),
                multiIf(startsWith(k, '@bytes@'), 'bytes',
                        startsWith(k, '@map@'),   'map',
                        startsWith(k, '@slice@'), 'slice', ''),
                'span'
            ), {{ .Layout.Keys "complex_attributes" }}),
            arrayMap(k -> (
                multiIf(startsWith(k, '@bytes@'), substring(k, 8),
                        startsWith(k, '@map@'),   substring(k, 6),
                        startsWith(k, '@slice@'), substring(k, 8), k),
                multiIf(startsWith(k, '@bytes@'), 'bytes',
                        startsWith(k, '@map@'),   'map',
                        startsWith(k, '@slice@'), 'slice', ''),
                'resource'
            ), {{ .Layout.Keys "resource_complex_attributes" }}),
            arrayMap(k -> (
                multiIf(startsWith(k, '@bytes@'), substring(k, 8),
                        startsWith(k, '@map@'),   substring(k, 6),
                        startsWith(k, '@slice@'), substring(k, 8), k),
                multiIf(startsWith(k, '@bytes@'), 'bytes',
                        startsWith(k, '@map@'),   'map',
                        startsWith(k, '@slice@'), 'slice', ''),
                'scope'
            ), {{ .Layout.Keys "scope_complex_attributes" }})
        )) AS tp
    FROM spans
)
GROUP BY attribute_key, type, level;
