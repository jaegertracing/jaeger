-- spans table
CREATE TABLE
    IF NOT EXISTS spans (
        id String,
        trace_id String,
        trace_state String,
        parent_span_id String,
        name String,
        kind String,
        start_time DateTime64 (9),
        status_code String,
        status_message String,
        duration Int64,
        bool_attributes Nested (key String, value Bool),
        double_attributes Nested (key String, value Float64),
        int_attributes Nested (key String, value Int64),
        str_attributes Nested (key String, value String),
        complex_attributes Nested (key String, value String),
        events Nested (
            name String,
            timestamp DateTime64 (9),
            bool_attributes Nested (key String, value Bool),
            double_attributes Nested (key String, value Float64),
            int_attributes Nested (key String, value Int64),
            str_attributes Nested (key String, value String),
            complex_attributes Nested (key String, value String)
        ),
        links Nested (
            trace_id String,
            span_id String,
            trace_state String,
            bool_attributes Nested (key String, value Bool),
            double_attributes Nested (key String, value Float64),
            int_attributes Nested (key String, value Int64),
            str_attributes Nested (key String, value String),
            complex_attributes Nested (key String, value String)
        ),
        service_name String,
        resource_bool_attributes Nested (key String, value Bool),
        resource_double_attributes Nested (key String, value Float64),
        resource_int_attributes Nested (key String, value Int64),
        resource_str_attributes Nested (key String, value String),
        resource_complex_attributes Nested (key String, value String),
        scope_name String,
        scope_version String,
        scope_bool_attributes Nested (key String, value Bool),
        scope_double_attributes Nested (key String, value Float64),
        scope_int_attributes Nested (key String, value Int64),
        scope_str_attributes Nested (key String, value String),
        scope_complex_attributes Nested (key String, value String),
        INDEX idx_trace_id trace_id TYPE bloom_filter GRANULARITY 1,
        INDEX idx_duration duration TYPE minmax GRANULARITY 1
    ) ENGINE = MergeTree
PARTITION BY toDate(start_time)
ORDER BY (service_name, name, toDateTime(start_time))

-- services table
CREATE TABLE
    IF NOT EXISTS services (name String) ENGINE = AggregatingMergeTree
ORDER BY
    (name);

-- services materialized view
CREATE MATERIALIZED VIEW IF NOT EXISTS services_mv TO services AS
SELECT
    service_name AS name
FROM
    spans
GROUP BY
    service_name;

-- operations table
CREATE TABLE IF NOT EXISTS
    operations (
        service_name String,
        name String,
        span_kind String
    ) ENGINE = AggregatingMergeTree
ORDER BY
    (service_name, span_kind);

-- operations materialized view
CREATE MATERIALIZED VIEW IF NOT EXISTS operations_mv TO operations AS
SELECT
    name,
    kind AS span_kind,
    service_name
FROM
    spans;

-- trace id timestamps table
CREATE TABLE IF NOT EXISTS trace_id_timestamps
(
    trace_id String,
    start SimpleAggregateFunction(min, DateTime64(9)),
    end SimpleAggregateFunction(max, DateTime64(9))
)
ENGINE = AggregatingMergeTree
ORDER BY (trace_id)

-- trace id timestamps materialized view
CREATE MATERIALIZED VIEW IF NOT EXISTS trace_id_timestamps_mv
TO trace_id_timestamps
AS
SELECT
    trace_id,
    min(start_time) AS start,
    max(addNanoseconds(start_time, duration)) AS end
FROM spans
GROUP BY trace_id;

-- attribute metadata table
CREATE TABLE
    IF NOT EXISTS attribute_metadata (
        attribute_key String,
        type String,  -- 'bool', 'double', 'int', 'str', 'bytes', 'map', 'slice'
        level String  -- 'resource', 'scope', 'span'
    ) ENGINE = AggregatingMergeTree
ORDER BY (attribute_key, type, level)

-- attribute metadata materialized view
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
                multiIf(startsWith(k, '@bytes@'), substring(k, 8),
                        startsWith(k, '@map@'),   substring(k, 6),
                        startsWith(k, '@slice@'), substring(k, 8), k),
                multiIf(startsWith(k, '@bytes@'), 'bytes',
                        startsWith(k, '@map@'),   'map',
                        startsWith(k, '@slice@'), 'slice', ''),
                'span'
            ), complex_attributes.key),
            arrayMap(k -> (
                multiIf(startsWith(k, '@bytes@'), substring(k, 8),
                        startsWith(k, '@map@'),   substring(k, 6),
                        startsWith(k, '@slice@'), substring(k, 8), k),
                multiIf(startsWith(k, '@bytes@'), 'bytes',
                        startsWith(k, '@map@'),   'map',
                        startsWith(k, '@slice@'), 'slice', ''),
                'resource'
            ), resource_complex_attributes.key),
            arrayMap(k -> (
                multiIf(startsWith(k, '@bytes@'), substring(k, 8),
                        startsWith(k, '@map@'),   substring(k, 6),
                        startsWith(k, '@slice@'), substring(k, 8), k),
                multiIf(startsWith(k, '@bytes@'), 'bytes',
                        startsWith(k, '@map@'),   'map',
                        startsWith(k, '@slice@'), 'slice', ''),
                'scope'
            ), scope_complex_attributes.key)
        )) AS tp
    FROM spans
)
GROUP BY attribute_key, type, level;

-- event attribute metadata materialized view
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
ARRAY JOIN arrayConcat(
    arrayMap(k -> (k, 'bool'),   arrayFlatten(events.bool_attributes.key)),
    arrayMap(k -> (k, 'double'), arrayFlatten(events.double_attributes.key)),
    arrayMap(k -> (k, 'int'),    arrayFlatten(events.int_attributes.key)),
    arrayMap(k -> (k, 'str'),    arrayFlatten(events.str_attributes.key)),
    arrayMap(k -> (
        multiIf(startsWith(k, '@bytes@'), substring(k, 8),
                startsWith(k, '@map@'),   substring(k, 6),
                startsWith(k, '@slice@'), substring(k, 8), k),
        multiIf(startsWith(k, '@bytes@'), 'bytes',
                startsWith(k, '@map@'),   'map',
                startsWith(k, '@slice@'), 'slice', '')
    ), arrayFlatten(events.complex_attributes.key))
) AS tp
GROUP BY attribute_key, type, level;

-- link attribute metadata materialized view
CREATE MATERIALIZED VIEW IF NOT EXISTS link_attribute_metadata_mv TO attribute_metadata AS
SELECT
    tp.1 AS attribute_key,
    tp.2 AS type,
    'link' AS level
FROM spans
ARRAY JOIN arrayConcat(
    arrayMap(k -> (k, 'bool'),   arrayFlatten(links.bool_attributes.key)),
    arrayMap(k -> (k, 'double'), arrayFlatten(links.double_attributes.key)),
    arrayMap(k -> (k, 'int'),    arrayFlatten(links.int_attributes.key)),
    arrayMap(k -> (k, 'str'),    arrayFlatten(links.str_attributes.key)),
    arrayMap(k -> (
        multiIf(startsWith(k, '@bytes@'), substring(k, 8),
                startsWith(k, '@map@'),   substring(k, 6),
                startsWith(k, '@slice@'), substring(k, 8), k),
        multiIf(startsWith(k, '@bytes@'), 'bytes',
                startsWith(k, '@map@'),   'map',
                startsWith(k, '@slice@'), 'slice', '')
    ), arrayFlatten(links.complex_attributes.key))
) AS tp
GROUP BY attribute_key, type, level;

-- dependencies table
CREATE TABLE
    IF NOT EXISTS dependencies (
        timestamp DateTime64 (9),
        dependencies_json String
    ) ENGINE = MergeTree
PARTITION BY
    toDate(timestamp)
ORDER BY
    (timestamp);
