// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

// Operation represents a single row in the ClickHouse `operations` table.
type Operation struct {
	Name string `ch:"name"`
	// SpanKind holds the string representation of the span kind from ptrace.SpanKind
	// in lowercase.
	SpanKind string `ch:"span_kind"`
}
