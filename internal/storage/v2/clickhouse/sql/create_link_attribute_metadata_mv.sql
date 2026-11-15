CREATE MATERIALIZED VIEW IF NOT EXISTS link_attribute_metadata_mv TO attribute_metadata AS
SELECT
    tp.1 AS attribute_key,
    tp.2 AS type,
    'link' AS level
FROM spans
ARRAY JOIN arrayConcat(
    arrayMap(k -> (k, 'bool'),   arrayFlatten({{ .Keys2DFn "links.bool_attributes" }})),
    arrayMap(k -> (k, 'double'), arrayFlatten({{ .Keys2DFn "links.double_attributes" }})),
    arrayMap(k -> (k, 'int'),    arrayFlatten({{ .Keys2DFn "links.int_attributes" }})),
    arrayMap(k -> (k, 'str'),    arrayFlatten({{ .Keys2DFn "links.str_attributes" }})),
    arrayMap(k -> (
        multiIf(startsWith(k, '@bytes@'), substring(k, 8),
                startsWith(k, '@map@'),   substring(k, 6),
                startsWith(k, '@slice@'), substring(k, 8), k),
        multiIf(startsWith(k, '@bytes@'), 'bytes',
                startsWith(k, '@map@'),   'map',
                startsWith(k, '@slice@'), 'slice', '')
    ), arrayFlatten({{ .Keys2DFn "links.complex_attributes" }}))
) AS tp
GROUP BY attribute_key, type, level;
