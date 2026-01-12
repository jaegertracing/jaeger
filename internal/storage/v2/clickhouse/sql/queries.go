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

const TruncateSpans = `TRUNCATE TABLE spans`

const TruncateServices = `TRUNCATE TABLE services`

const TruncateOperations = `TRUNCATE TABLE operations`

const TruncateTraceIDTimestamps = `TRUNCATE TABLE trace_id_timestamps`

const TruncateAttributeMetadata = `TRUNCATE TABLE attribute_metadata`

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
