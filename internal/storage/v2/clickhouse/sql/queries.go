// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sql

import _ "embed"

const InsertSpan = `
INSERT INTO
    spans (
        id,
        trace_id,
        trace_state,
        parent_span_id,
        name,
        kind,
        start_time,
        status_code,
        status_message,
        duration,
        bool_attributes.key,
        bool_attributes.value,
        double_attributes.key,
        double_attributes.value,
        int_attributes.key,
        int_attributes.value,
        str_attributes.key,
        str_attributes.value,
        complex_attributes.key,
        complex_attributes.value,
        events.name,
        events.timestamp,
        events.bool_attributes,
        events.double_attributes,
        events.int_attributes,
        events.str_attributes,
        events.complex_attributes,
        links.trace_id,
        links.span_id,
        links.trace_state,
        links.bool_attributes,
        links.double_attributes,
        links.int_attributes,
        links.str_attributes,
        links.complex_attributes,
        service_name,
        resource_bool_attributes.key,
        resource_bool_attributes.value,
        resource_double_attributes.key,
        resource_double_attributes.value,
        resource_int_attributes.key,
        resource_int_attributes.value,
        resource_str_attributes.key,
        resource_str_attributes.value,
        resource_complex_attributes.key,
        resource_complex_attributes.value,
        scope_name,
        scope_version,
        scope_bool_attributes.key,
        scope_bool_attributes.value,
        scope_double_attributes.key,
        scope_double_attributes.value,
        scope_int_attributes.key,
        scope_int_attributes.value,
        scope_str_attributes.key,
        scope_str_attributes.value,
        scope_complex_attributes.key,
        scope_complex_attributes.value
    )
VALUES
    (
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?,
        ?
    )
`

const SelectSpansQuery = `
SELECT
    id,
    trace_id,
    trace_state,
    parent_span_id,
    name,
    kind,
    start_time,
    status_code,
    status_message,
    duration,
    bool_attributes.key,
    bool_attributes.value,
    double_attributes.key,
    double_attributes.value,
    int_attributes.key,
    int_attributes.value,
    str_attributes.key,
    str_attributes.value,
    complex_attributes.key,
    complex_attributes.value,
    events.name,
    events.timestamp,
    events.bool_attributes.key,
    events.bool_attributes.value,
    events.double_attributes.key,
    events.double_attributes.value,
    events.int_attributes.key,
    events.int_attributes.value,
    events.str_attributes.key,
    events.str_attributes.value,
    events.complex_attributes.key,
    events.complex_attributes.value,
    links.trace_id,
    links.span_id,
    links.trace_state,
    links.bool_attributes.key,
    links.bool_attributes.value,
    links.double_attributes.key,
    links.double_attributes.value,
    links.int_attributes.key,
    links.int_attributes.value,
    links.str_attributes.key,
    links.str_attributes.value,
    links.complex_attributes.key,
    links.complex_attributes.value,
    service_name,
    resource_bool_attributes.key,
    resource_bool_attributes.value,
    resource_double_attributes.key,
    resource_double_attributes.value,
    resource_int_attributes.key,
    resource_int_attributes.value,
    resource_str_attributes.key,
    resource_str_attributes.value,
    resource_complex_attributes.key,
    resource_complex_attributes.value,
    scope_name,
    scope_version,
    scope_bool_attributes.key,
    scope_bool_attributes.value,
    scope_double_attributes.key,
    scope_double_attributes.value,
    scope_int_attributes.key,
    scope_int_attributes.value,
    scope_str_attributes.key,
    scope_str_attributes.value,
    scope_complex_attributes.key,
    scope_complex_attributes.value
FROM
    spans s
`

const SelectSpansByTraceID = SelectSpansQuery + " WHERE s.trace_id = ?"

// SearchTraceIDsBase is the inner SQL fragment for finding distinct trace IDs.
//
// The query begins with a no-op predicate (`WHERE 1=1`) so that additional
// filters can be appended unconditionally using `AND` without needing to check
// whether this is the first WHERE clause.
const SearchTraceIDsBase = `SELECT DISTINCT
    s.trace_id
FROM spans s
WHERE 1=1`

// SearchTraceIDs wraps a trace ID subquery with a JOIN to
// trace_id_timestamps to retrieve the start and end times for each trace.
// The %s placeholder is replaced with the complete inner subquery
// (SearchTraceIDsBase + conditions + LIMIT).
const SearchTraceIDs = `
SELECT
    l.trace_id,
    min(t.start) AS start,
    max(t.end) AS end
FROM (
%s
) l
LEFT JOIN trace_id_timestamps t ON l.trace_id = t.trace_id
GROUP BY l.trace_id`

// SelectTraceSummaries computes lightweight per-trace summaries natively in
// ClickHouse, avoiding materialization of full span payloads. The %s placeholder
// is replaced with the limited trace-ID subquery (SearchTraceIDsBase + filters +
// LIMIT), so only the already-selected traces are aggregated.
//
// The inner query aggregates one row per span into per-trace statistics:
//   - min_start / max_end derive the trace duration without fetching spans.
//   - root_service / root_operation come from the parentless span (empty
//     parent_span_id) with the earliest start time, via argMinIf, matching the
//     client-side fallback in querysvc.summarizeTrace.
//   - svc_stats is a single multi-value sumMap keyed by service_name carrying
//     both per-service span and error counts. A single sumMap guarantees one
//     shared, sorted key array with both value arrays aligned to it; using two
//     separate sumMaps would misalign, because sumMap drops keys whose summed
//     value is zero (an error-free service would vanish from the error map).
//   - span_ids / parent_ids feed the outer orphan-span computation.
//
// The outer query projects the sumMap tuple into parallel arrays and counts
// orphan spans (a span whose parent is absent from the trace).
const SelectTraceSummaries = `
SELECT
    trace_id,
    min_start,
    max_end,
    span_count,
    error_span_count,
    root_service,
    root_operation,
    svc_stats.1 AS svc_names,
    svc_stats.2 AS svc_span_counts,
    svc_stats.3 AS svc_error_counts,
    toUInt64(arrayCount(p -> p != '' AND NOT has(span_ids, p), parent_ids)) AS orphan_count
FROM (
    SELECT
        s.trace_id AS trace_id,
        min(s.start_time) AS min_start,
        max(s.start_time + toIntervalNanosecond(s.duration)) AS max_end,
        toUInt64(count()) AS span_count,
        toUInt64(countIf(s.status_code = 'Error')) AS error_span_count,
        argMinIf(s.service_name, s.start_time, s.parent_span_id = '') AS root_service,
        argMinIf(s.name, s.start_time, s.parent_span_id = '') AS root_operation,
        sumMap([s.service_name], [toUInt64(1)], [toUInt64(s.status_code = 'Error')]) AS svc_stats,
        groupArray(s.id) AS span_ids,
        groupArray(s.parent_span_id) AS parent_ids
    FROM spans s
    WHERE s.trace_id IN (
        SELECT trace_id FROM (
%s
        )
    )
    GROUP BY s.trace_id
)
ORDER BY min_start DESC`

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
