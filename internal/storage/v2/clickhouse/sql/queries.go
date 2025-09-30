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
        service_name,
        scope_name,
        scope_version
    )
VALUES
    (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
    service_name,
    scope_name,
    scope_version
FROM
    spans
WHERE
    trace_id = ?
`

const SelectServices = `
SELECT DISTINCT
    name
FROM
    services
`

const SelectOperationsAllKinds = `
SELECT
    name,
    span_kind
FROM
    operations
WHERE
    service_name = ?
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
`

//go:embed create_schema.sql
var CreateSchema string
