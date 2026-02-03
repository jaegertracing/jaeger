CREATE MATERIALIZED VIEW IF NOT EXISTS link_attribute_metadata_mv TO attribute_metadata AS
SELECT
    tp.1 AS attribute_key,
    tp.2 AS type,
    'link' AS level
FROM spans
ARRAY JOIN links AS l
ARRAY JOIN arrayConcat(
    arrayMap(k -> (k, 'bool'),   l.bool_attributes.key),
    arrayMap(k -> (k, 'double'), l.double_attributes.key),
    arrayMap(k -> (k, 'int'),    l.int_attributes.key),
    arrayMap(k -> (k, 'str'),    l.str_attributes.key),
    arrayMap(k -> (
        multiIf(startsWith(k, '@bytes@'), substring(k, 8), 
                startsWith(k, '@map@'),   substring(k, 6), 
                startsWith(k, '@slice@'), substring(k, 8), k),
        multiIf(startsWith(k, '@bytes@'), 'bytes', 
                startsWith(k, '@map@'),   'map', 
                startsWith(k, '@slice@'), 'slice', '')
    ), l.complex_attributes.key)
) AS tp
GROUP BY attribute_key, type, level;
