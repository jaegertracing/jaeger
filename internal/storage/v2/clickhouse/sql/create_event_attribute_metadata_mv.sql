-- In ClickHouse, 'events' are a nested array. If we processed span-level attributes and 
-- event-level attributes in the same materialized view, the ARRAY JOIN on events would create 
-- a separate row for each event in each span, causing span-level attributes to be duplicated 
-- for every event.
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