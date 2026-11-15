CREATE MATERIALIZED VIEW IF NOT EXISTS attribute_metadata_mv TO attribute_metadata AS
SELECT
    tp.1 AS attribute_key,
    tp.2 AS type,
    tp.3 AS level
FROM (
    SELECT
        arrayJoin(arrayConcat(
            arrayMap(k -> (k, 'bool',   'span'),     {{ .Layout.KeysSQL "bool_attributes" }}),
            arrayMap(k -> (k, 'bool',   'resource'), {{ .Layout.KeysSQL "resource_bool_attributes" }}),
            arrayMap(k -> (k, 'bool',   'scope'),    {{ .Layout.KeysSQL "scope_bool_attributes" }}),
            arrayMap(k -> (k, 'double', 'span'),     {{ .Layout.KeysSQL "double_attributes" }}),
            arrayMap(k -> (k, 'double', 'resource'), {{ .Layout.KeysSQL "resource_double_attributes" }}),
            arrayMap(k -> (k, 'double', 'scope'),    {{ .Layout.KeysSQL "scope_double_attributes" }}),
            arrayMap(k -> (k, 'int',    'span'),     {{ .Layout.KeysSQL "int_attributes" }}),
            arrayMap(k -> (k, 'int',    'resource'), {{ .Layout.KeysSQL "resource_int_attributes" }}),
            arrayMap(k -> (k, 'int',    'scope'),    {{ .Layout.KeysSQL "scope_int_attributes" }}),
            arrayMap(k -> (k, 'str',    'span'),     {{ .Layout.KeysSQL "str_attributes" }}),
            arrayMap(k -> (k, 'str',    'resource'), {{ .Layout.KeysSQL "resource_str_attributes" }}),
            arrayMap(k -> (k, 'str',    'scope'),    {{ .Layout.KeysSQL "scope_str_attributes" }}),
            arrayMap(k -> (
                multiIf(startsWith(k, '@bytes@'), substring(k, 8),
                        startsWith(k, '@map@'),   substring(k, 6),
                        startsWith(k, '@slice@'), substring(k, 8), k),
                multiIf(startsWith(k, '@bytes@'), 'bytes',
                        startsWith(k, '@map@'),   'map',
                        startsWith(k, '@slice@'), 'slice', ''),
                'span'
            ), {{ .Layout.KeysSQL "complex_attributes" }}),
            arrayMap(k -> (
                multiIf(startsWith(k, '@bytes@'), substring(k, 8),
                        startsWith(k, '@map@'),   substring(k, 6),
                        startsWith(k, '@slice@'), substring(k, 8), k),
                multiIf(startsWith(k, '@bytes@'), 'bytes',
                        startsWith(k, '@map@'),   'map',
                        startsWith(k, '@slice@'), 'slice', ''),
                'resource'
            ), {{ .Layout.KeysSQL "resource_complex_attributes" }}),
            arrayMap(k -> (
                multiIf(startsWith(k, '@bytes@'), substring(k, 8),
                        startsWith(k, '@map@'),   substring(k, 6),
                        startsWith(k, '@slice@'), substring(k, 8), k),
                multiIf(startsWith(k, '@bytes@'), 'bytes',
                        startsWith(k, '@map@'),   'map',
                        startsWith(k, '@slice@'), 'slice', ''),
                'scope'
            ), {{ .Layout.KeysSQL "scope_complex_attributes" }})
        )) AS tp
    FROM spans
)
GROUP BY attribute_key, type, level;
