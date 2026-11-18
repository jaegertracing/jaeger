CREATE MATERIALIZED VIEW IF NOT EXISTS attribute_metadata_mv TO attribute_metadata{{ .LocalTableSuffix }} AS
SELECT
    tp.1 AS attribute_key,
    tp.2 AS type,
    tp.3 AS level
FROM (
    SELECT
        arrayJoin(arrayConcat(
            arrayMap(k -> (k, 'bool',   'span'),     {{ .KeysFn "span" "bool" }}),
            arrayMap(k -> (k, 'bool',   'resource'), {{ .KeysFn "resource" "bool" }}),
            arrayMap(k -> (k, 'bool',   'scope'),    {{ .KeysFn "scope" "bool" }}),
            arrayMap(k -> (k, 'double', 'span'),     {{ .KeysFn "span" "double" }}),
            arrayMap(k -> (k, 'double', 'resource'), {{ .KeysFn "resource" "double" }}),
            arrayMap(k -> (k, 'double', 'scope'),    {{ .KeysFn "scope" "double" }}),
            arrayMap(k -> (k, 'int',    'span'),     {{ .KeysFn "span" "int" }}),
            arrayMap(k -> (k, 'int',    'resource'), {{ .KeysFn "resource" "int" }}),
            arrayMap(k -> (k, 'int',    'scope'),    {{ .KeysFn "scope" "int" }}),
            arrayMap(k -> (k, 'str',    'span'),     {{ .KeysFn "span" "str" }}),
            arrayMap(k -> (k, 'str',    'resource'), {{ .KeysFn "resource" "str" }}),
            arrayMap(k -> (k, 'str',    'scope'),    {{ .KeysFn "scope" "str" }}),
            arrayMap(k -> (
                multiIf(startsWith(k, '@bytes@'), substring(k, 8),
                        startsWith(k, '@map@'),   substring(k, 6),
                        startsWith(k, '@slice@'), substring(k, 8), k),
                multiIf(startsWith(k, '@bytes@'), 'bytes',
                        startsWith(k, '@map@'),   'map',
                        startsWith(k, '@slice@'), 'slice', ''),
                'span'
            ), {{ .KeysFn "span" "complex" }}),
            arrayMap(k -> (
                multiIf(startsWith(k, '@bytes@'), substring(k, 8),
                        startsWith(k, '@map@'),   substring(k, 6),
                        startsWith(k, '@slice@'), substring(k, 8), k),
                multiIf(startsWith(k, '@bytes@'), 'bytes',
                        startsWith(k, '@map@'),   'map',
                        startsWith(k, '@slice@'), 'slice', ''),
                'resource'
            ), {{ .KeysFn "resource" "complex" }}),
            arrayMap(k -> (
                multiIf(startsWith(k, '@bytes@'), substring(k, 8),
                        startsWith(k, '@map@'),   substring(k, 6),
                        startsWith(k, '@slice@'), substring(k, 8), k),
                multiIf(startsWith(k, '@bytes@'), 'bytes',
                        startsWith(k, '@map@'),   'map',
                        startsWith(k, '@slice@'), 'slice', ''),
                'scope'
            ), {{ .KeysFn "scope" "complex" }})
        )) AS tp
    FROM spans{{ .LocalTableSuffix }}
)
GROUP BY attribute_key, type, level;
