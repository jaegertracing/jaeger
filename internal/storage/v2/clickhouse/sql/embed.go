// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sql

import _ "embed"

//go:embed insert_span.sql
var InsertSpan string

//go:embed select_spans_by_trace_id.sql
var SelectSpansByTraceID string

//go:embed select_services.sql
var SelectServices string

//go:embed select_operations_all_kinds.sql
var SelectOperationsAllKinds string

//go:embed select_operations_by_kind.sql
var SelectOperationsByKind string
