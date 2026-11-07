-- Copyright (c) 2026 The Jaeger Authors.
-- SPDX-License-Identifier: Apache-2.0
--
-- The multi-node counterpart of Jaeger's create_schema. Jaeger runs with create_schema: false
-- against these Distributed wrappers. The materialized views are attached to the local tables,
-- so each replica maintains its own derived rows; reads go through the wrappers.

CREATE DATABASE IF NOT EXISTS jaeger ON CLUSTER jaeger_cluster;

CREATE TABLE IF NOT EXISTS jaeger.spans_local ON CLUSTER jaeger_cluster
(
    id String,
    trace_id String,
    trace_state String,
    parent_span_id String,
    name String,
    kind String,
    start_time DateTime64(9),
    status_code String,
    status_message String,
    duration Int64,
    bool_attributes Nested(key String, value Bool),
    double_attributes Nested(key String, value Float64),
    int_attributes Nested(key String, value Int64),
    str_attributes Nested(key String, value String),
    complex_attributes Nested(key String, value String),
    events Nested(
        name String,
        timestamp DateTime64(9),
        bool_attributes Nested(key String, value Bool),
        double_attributes Nested(key String, value Float64),
        int_attributes Nested(key String, value Int64),
        str_attributes Nested(key String, value String),
        complex_attributes Nested(key String, value String)
    ),
    links Nested(
        trace_id String,
        span_id String,
        trace_state String,
        bool_attributes Nested(key String, value Bool),
        double_attributes Nested(key String, value Float64),
        int_attributes Nested(key String, value Int64),
        str_attributes Nested(key String, value String),
        complex_attributes Nested(key String, value String)
    ),
    service_name String,
    resource_bool_attributes Nested(key String, value Bool),
    resource_double_attributes Nested(key String, value Float64),
    resource_int_attributes Nested(key String, value Int64),
    resource_str_attributes Nested(key String, value String),
    resource_complex_attributes Nested(key String, value String),
    scope_name String,
    scope_version String,
    scope_bool_attributes Nested(key String, value Bool),
    scope_double_attributes Nested(key String, value Float64),
    scope_int_attributes Nested(key String, value Int64),
    scope_str_attributes Nested(key String, value String),
    scope_complex_attributes Nested(key String, value String),
    INDEX idx_trace_id trace_id TYPE bloom_filter GRANULARITY 1,
    INDEX idx_duration duration TYPE minmax GRANULARITY 1
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/{database}/spans_local', '{replica}')
PARTITION BY toDate(start_time)
ORDER BY (service_name, name, toDateTime(start_time));

CREATE TABLE IF NOT EXISTS jaeger.spans ON CLUSTER jaeger_cluster
AS jaeger.spans_local
ENGINE = Distributed(jaeger_cluster, jaeger, spans_local, cityHash64(trace_id));

CREATE TABLE IF NOT EXISTS jaeger.trace_id_timestamps_local ON CLUSTER jaeger_cluster
(
    trace_id String,
    start SimpleAggregateFunction(min, DateTime64(9)),
    end SimpleAggregateFunction(max, DateTime64(9))
)
ENGINE = ReplicatedAggregatingMergeTree('/clickhouse/tables/{shard}/{database}/trace_id_timestamps_local', '{replica}')
ORDER BY (trace_id)
SETTINGS index_granularity = 4;

CREATE TABLE IF NOT EXISTS jaeger.trace_id_timestamps ON CLUSTER jaeger_cluster
AS jaeger.trace_id_timestamps_local
ENGINE = Distributed(jaeger_cluster, jaeger, trace_id_timestamps_local, cityHash64(trace_id));

CREATE TABLE IF NOT EXISTS jaeger.dependencies_local ON CLUSTER jaeger_cluster
(
    timestamp DateTime64(9),
    dependencies_json String
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/{database}/dependencies_local', '{replica}')
PARTITION BY toDate(timestamp)
ORDER BY (timestamp);

CREATE TABLE IF NOT EXISTS jaeger.dependencies ON CLUSTER jaeger_cluster
AS jaeger.dependencies_local
ENGINE = Distributed(jaeger_cluster, jaeger, dependencies_local, cityHash64(timestamp));

CREATE TABLE IF NOT EXISTS jaeger.services_local ON CLUSTER jaeger_cluster
(
    name String
)
ENGINE = ReplicatedAggregatingMergeTree('/clickhouse/tables/{shard}/{database}/services_local', '{replica}')
ORDER BY (name);

CREATE TABLE IF NOT EXISTS jaeger.services ON CLUSTER jaeger_cluster
AS jaeger.services_local
ENGINE = Distributed(jaeger_cluster, jaeger, services_local, cityHash64(name));

CREATE TABLE IF NOT EXISTS jaeger.operations_local ON CLUSTER jaeger_cluster
(
    service_name String,
    name String,
    span_kind String
)
ENGINE = ReplicatedAggregatingMergeTree('/clickhouse/tables/{shard}/{database}/operations_local', '{replica}')
ORDER BY (service_name, span_kind);

CREATE TABLE IF NOT EXISTS jaeger.operations ON CLUSTER jaeger_cluster
AS jaeger.operations_local
ENGINE = Distributed(jaeger_cluster, jaeger, operations_local, cityHash64(service_name));

CREATE TABLE IF NOT EXISTS jaeger.attribute_metadata_local ON CLUSTER jaeger_cluster
(
    attribute_key String,
    type String,
    level String
)
ENGINE = ReplicatedAggregatingMergeTree('/clickhouse/tables/{shard}/{database}/attribute_metadata_local', '{replica}')
ORDER BY (attribute_key, type, level);

CREATE TABLE IF NOT EXISTS jaeger.attribute_metadata ON CLUSTER jaeger_cluster
AS jaeger.attribute_metadata_local
ENGINE = Distributed(jaeger_cluster, jaeger, attribute_metadata_local, cityHash64(attribute_key));

CREATE MATERIALIZED VIEW IF NOT EXISTS jaeger.trace_id_timestamps_mv ON CLUSTER jaeger_cluster
TO jaeger.trace_id_timestamps_local
AS
SELECT
    trace_id,
    min(start_time) AS start,
    max(start_time) AS end
FROM jaeger.spans_local
GROUP BY trace_id;

CREATE MATERIALIZED VIEW IF NOT EXISTS jaeger.services_mv ON CLUSTER jaeger_cluster
TO jaeger.services_local
AS
SELECT service_name AS name
FROM jaeger.spans_local
GROUP BY service_name;

CREATE MATERIALIZED VIEW IF NOT EXISTS jaeger.operations_mv ON CLUSTER jaeger_cluster
TO jaeger.operations_local
AS
SELECT
    name,
    kind AS span_kind,
    service_name
FROM jaeger.spans_local;

CREATE MATERIALIZED VIEW IF NOT EXISTS jaeger.attribute_metadata_mv ON CLUSTER jaeger_cluster
TO jaeger.attribute_metadata_local
AS
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
    FROM jaeger.spans_local
)
GROUP BY attribute_key, type, level;

-- Events and links have their own views so the ARRAY JOIN does not multiply the span's other
-- attribute rows by the number of events or links.
CREATE MATERIALIZED VIEW IF NOT EXISTS jaeger.event_attribute_metadata_mv ON CLUSTER jaeger_cluster
TO jaeger.attribute_metadata_local
AS
SELECT
    tp.1 AS attribute_key,
    tp.2 AS type,
    'event' AS level
FROM jaeger.spans_local
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

CREATE MATERIALIZED VIEW IF NOT EXISTS jaeger.link_attribute_metadata_mv ON CLUSTER jaeger_cluster
TO jaeger.attribute_metadata_local
AS
SELECT
    tp.1 AS attribute_key,
    tp.2 AS type,
    'link' AS level
FROM jaeger.spans_local
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
