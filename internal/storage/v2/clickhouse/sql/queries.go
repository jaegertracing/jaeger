// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sql

import _ "embed"

// SearchTraceIDsBase is the inner SQL fragment for finding distinct trace IDs.
//
// The query begins with a no-op predicate (`WHERE 1=1`) so that additional
// filters can be appended unconditionally using `AND` without needing to check
// whether this is the first WHERE clause.
const SearchTraceIDsBase = `SELECT DISTINCT
    s.trace_id
FROM spans s
WHERE 1=1`

// SelectTraceIDTimestamps looks up global trace bounds only for the selected IDs.
const SelectTraceIDTimestamps = `SELECT
    trace_id,
    min(start) AS start,
    max(end) AS end
FROM trace_id_timestamps
WHERE trace_id IN (?)
GROUP BY trace_id`

const SelectServices = `
SELECT
    name
FROM
    services
GROUP BY name
`

const SelectOperationsAllKinds = `
SELECT
    name,
    span_kind
FROM
    operations
WHERE
    service_name = ?
GROUP BY name, span_kind
`

const SelectOperationsByKind = `
SELECT
    name,
    span_kind
FROM
    operations
WHERE
    service_name = ?
    AND span_kind = ?
GROUP BY name, span_kind
`

const SelectAttributeMetadata = `
SELECT
    attribute_key,
    type,
    level
FROM
    attribute_metadata`

// SelectHoistedColumn reads how the spans table declares one column. The server formats the
// stored expression and the configured one before it compares their text, so a difference in
// spacing or in how a string literal is escaped does not count as a difference. An ordinary
// column has no expression to format, which formatQuerySingleLineOrNull reports as NULL and
// not as an error.
const SelectHoistedColumn = `
SELECT
    type,
    default_kind,
    default_expression,
    compression_codec,
    ifNull(formatQuerySingleLineOrNull(concat('SELECT ', default_expression)) = formatQuerySingleLine(concat('SELECT ', ?)), 0) AS same_expression
FROM system.columns
WHERE database = currentDatabase() AND table = 'spans' AND name = ?`

// SelectAttributeLayout returns one row when the spans table exists, which says whether the
// table stores the str_attributes group as a Map. In the nested layout the server lists the
// group as str_attributes.key, or as str_attributes when flatten_nested is off.
const SelectAttributeLayout = `
SELECT startsWith(type, 'Map(') AS is_map
FROM system.columns
WHERE database = currentDatabase() AND table = 'spans' AND name IN ('str_attributes', 'str_attributes.key')`

// SelectSpansTable returns one row when the current database has a spans table.
const SelectSpansTable = `SELECT 1 FROM system.tables WHERE database = currentDatabase() AND name = 'spans'`

const (
	TruncateSpans             = `TRUNCATE TABLE IF EXISTS spans`
	TruncateServices          = `TRUNCATE TABLE IF EXISTS services`
	TruncateOperations        = `TRUNCATE TABLE IF EXISTS operations`
	TruncateTraceIDTimestamps = `TRUNCATE TABLE IF EXISTS trace_id_timestamps`
	TruncateAttributeMetadata = `TRUNCATE TABLE IF EXISTS attribute_metadata`
	TruncateDependencies      = `TRUNCATE TABLE IF EXISTS dependencies`
)

const SelectDependencies = `
SELECT
    dependencies_json
FROM
    dependencies
WHERE
    timestamp >= ?
    AND timestamp < ?`

const InsertDependencies = `
INSERT INTO
    dependencies (timestamp, dependencies_json)
VALUES
    (?, ?)`

const SelectLatencies = `
SELECT
    toStartOfInterval(start_time, toIntervalSecond(?)) AS ts,
    service_name,
    quantile(?)(duration) / 1e6 AS val
FROM
    spans
WHERE
    start_time >= ?
    AND start_time < ?
    AND service_name IN (?)
    AND kind IN (?)
GROUP BY
    ts,
    service_name
ORDER BY
    ts`

const SelectLatenciesByOperation = `
SELECT
    toStartOfInterval(start_time, toIntervalSecond(?)) AS ts,
    service_name,
    name,
    quantile(?)(duration) / 1e6 AS val
FROM
    spans
WHERE
    start_time >= ?
    AND start_time < ?
    AND service_name IN (?)
    AND kind IN (?)
GROUP BY
    ts,
    service_name,
    name
ORDER BY
    ts`

const SelectCallRates = `
SELECT
    toStartOfInterval(start_time, toIntervalSecond(?)) AS ts,
    service_name,
    count(*) / ? AS val
FROM
    spans
WHERE
    start_time >= ?
    AND start_time < ?
    AND service_name IN (?)
    AND kind IN (?)
GROUP BY
    ts,
    service_name
ORDER BY
    ts`

const SelectCallRatesByOperation = `
SELECT
    toStartOfInterval(start_time, toIntervalSecond(?)) AS ts,
    service_name,
    name,
    count(*) / ? AS val
FROM
    spans
WHERE
    start_time >= ?
    AND start_time < ?
    AND service_name IN (?)
    AND kind IN (?)
GROUP BY
    ts,
    service_name,
    name
ORDER BY
    ts`

const SelectErrorRates = `
SELECT
    toStartOfInterval(start_time, toIntervalSecond(?)) AS ts,
    service_name,
    countIf(status_code = 'Error') / count(*) AS val
FROM
    spans
WHERE
    start_time >= ?
    AND start_time < ?
    AND service_name IN (?)
    AND kind IN (?)
GROUP BY
    ts,
    service_name
ORDER BY
    ts`

const SelectErrorRatesByOperation = `
SELECT
    toStartOfInterval(start_time, toIntervalSecond(?)) AS ts,
    service_name,
    name,
    countIf(status_code = 'Error') / count(*) AS val
FROM
    spans
WHERE
    start_time >= ?
    AND start_time < ?
    AND service_name IN (?)
    AND kind IN (?)
GROUP BY
    ts,
    service_name,
    name
ORDER BY
    ts`

//go:embed create_dependencies_table.sql
var CreateDependenciesTable string

//go:embed create_spans_table.sql
var CreateSpansTable string

//go:embed create_services_table.sql
var CreateServicesTable string

//go:embed create_services_mv.sql
var CreateServicesMaterializedView string

//go:embed create_operations_table.sql
var CreateOperationsTable string

//go:embed create_operations_mv.sql
var CreateOperationsMaterializedView string

//go:embed create_trace_id_timestamps_table.sql
var CreateTraceIDTimestampsTable string

//go:embed create_trace_id_timestamps_mv.sql
var CreateTraceIDTimestampsMaterializedView string

//go:embed create_attribute_metadata_table.sql
var CreateAttributeMetadataTable string

//go:embed create_attribute_metadata_mv.sql
var CreateAttributeMetadataMaterializedView string

//go:embed create_event_attribute_metadata_mv.sql
var CreateEventAttributeMetadataMaterializedView string

//go:embed create_link_attribute_metadata_mv.sql
var CreateLinkAttributeMetadataMaterializedView string
