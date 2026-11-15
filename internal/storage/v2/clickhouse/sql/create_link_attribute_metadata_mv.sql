CREATE MATERIALIZED VIEW IF NOT EXISTS link_attribute_metadata_mv TO attribute_metadata AS
SELECT
    tp.1 AS attribute_key,
    tp.2 AS type,
    'link' AS level
FROM spans
ARRAY JOIN arrayConcat(
    arrayMap(k -> (k, 'bool'),   arrayFlatten({{ .KeysFn "link" "bool" }})),
    arrayMap(k -> (k, 'double'), arrayFlatten({{ .KeysFn "link" "double" }})),
    arrayMap(k -> (k, 'int'),    arrayFlatten({{ .KeysFn "link" "int" }})),
    arrayMap(k -> (k, 'str'),    arrayFlatten({{ .KeysFn "link" "str" }})),
    arrayMap(k -> (
        multiIf(startsWith(k, '@bytes@'), substring(k, 8),
                startsWith(k, '@map@'),   substring(k, 6),
                startsWith(k, '@slice@'), substring(k, 8), k),
        multiIf(startsWith(k, '@bytes@'), 'bytes',
                startsWith(k, '@map@'),   'map',
                startsWith(k, '@slice@'), 'slice', '')
    ), arrayFlatten({{ .KeysFn "link" "complex" }}))
) AS tp
GROUP BY attribute_key, type, level;
