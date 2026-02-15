-- Events and links are stored in a nested list. If we processed them in the same view 
-- as the main span data, ClickHouse would have to create a "duplicate" row 
-- of the span for every single link found because of the ARRAY JOIN operation.
-- Example: A span with 10 attributes and 5 links would turn into 5 rows 
-- of 10 attributes each, forcing the database to process 50 items instead of 15.
CREATE MATERIALIZED VIEW IF NOT EXISTS event_attribute_metadata_mv TO attribute_metadata AS
SELECT
    tp.1 AS attribute_key,
    tp.2 AS type,
    'event' AS level
FROM spans
ARRAY JOIN events AS e
ARRAY JOIN arrayConcat(
    arrayMap(k -> (k, 'bool'),   e.bool_attributes.key),
    arrayMap(k -> (k, 'double'), e.double_attributes.key),
    arrayMap(k -> (k, 'int'),    e.int_attributes.key),
    arrayMap(k -> (k, 'str'),    e.str_attributes.key),
    arrayMap(k -> (
        multiIf(startsWith(k, '@bytes@'), substring(k, 8), 
                startsWith(k, '@map@'),   substring(k, 6), 
                startsWith(k, '@slice@'), substring(k, 8), k),
        multiIf(startsWith(k, '@bytes@'), 'bytes', 
                startsWith(k, '@map@'),   'map', 
                startsWith(k, '@slice@'), 'slice', '')
    ), e.complex_attributes.key)
) AS tp
GROUP BY attribute_key, type, level;