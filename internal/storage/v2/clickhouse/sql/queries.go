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
        attributes.key,
        attributes.value,
        attributes.type,
        events.name,
        events.timestamp,
        events.attributes,
        links.trace_id,
        links.span_id,
        links.trace_state,
        links.attributes,
        service_name,
        resource_attributes.key,
        resource_attributes.value,
        resource_attributes.type,
        scope_name,
        scope_version,
        scope_attributes.key,
        scope_attributes.value,
        scope_attributes.type,
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
    )
`

const SelectSpansByTraceID = `
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
    attributes.key,
    attributes.value,
    attributes.type,
    events.name,
    events.timestamp,
    events.attributes.key,
    events.attributes.value,
    events.attributes.type,
    links.trace_id,
    links.span_id,
    links.trace_state,
    links.attributes.key,
    links.attributes.value,
    links.attributes.type,
    service_name,
    resource_attributes.key,
    resource_attributes.value,
    resource_attributes.type,
    scope_name,
    scope_version,
    scope_attributes.key,
    scope_attributes.value,
    scope_attributes.type
FROM
    spans
WHERE
    trace_id = ?
`

// SearchTraces is the base SQL fragment used by FindTraces.
//
// The query begins with a no-op predicate (`WHERE 1=1`) so that additional
// filters can be appended unconditionally using `AND` without needing to check
// whether this is the first WHERE clause.
const SearchTraces = `
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
    attributes.key,
    attributes.value,
    attributes.type,
    events.name,
    events.timestamp,
    events.attributes.key,
    events.attributes.value,
    events.attributes.type,
    links.trace_id,
    links.span_id,
    links.trace_state,
    links.attributes.key,
    links.attributes.value,
    links.attributes.type,
    service_name,
    resource_attributes.key,
    resource_attributes.value,
    resource_attributes.type,
    scope_name,
    scope_version,
    scope_attributes.key,
    scope_attributes.value,
    scope_attributes.type
FROM
    spans s
WHERE 1=1
`

// SearchTraceIDs is the base SQL fragment used by FindTraceIDs.
//
// The query begins with a no-op predicate (`WHERE 1=1`) so that additional
// filters can be appended unconditionally using `AND` without needing to check
// whether this is the first WHERE clause.
//
// The query joins with trace_id_timestamps to retrieve the start and end times
// for each trace ID.
const SearchTraceIDs = `
SELECT DISTINCT
    s.trace_id,
    t.start,
    t.end
FROM spans s
LEFT JOIN trace_id_timestamps t ON s.trace_id = t.trace_id
WHERE 1=1`

const SelectServices = `
SELECT DISTINCT
    name
FROM
    services
`

const SelectOperationsAllKinds = `
SELECT DISTINCT
    name,
    span_kind
FROM
    operations
WHERE
    service_name = ?
`

const SelectOperationsByKind = `
SELECT DISTINCT
    name,
    span_kind
FROM
    operations
WHERE
    service_name = ?
    AND span_kind = ?
`

const TruncateSpans = `TRUNCATE TABLE spans`

const TruncateServices = `TRUNCATE TABLE services`

const TruncateOperations = `TRUNCATE TABLE operations`

const TruncateTraceIDTimestamps = `TRUNCATE TABLE trace_id_timestamps`

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
