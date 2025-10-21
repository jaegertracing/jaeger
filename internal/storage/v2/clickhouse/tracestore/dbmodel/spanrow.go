// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// SpanRow represents a single row in the ClickHouse `spans` table.
//
// Complex attributes are attributes that are not of a primitive type and hence need special handling.
// The following OTLP types are stored in the complex attributes fields:
//   - AnyValue_BytesValue: This OTLP type is stored as a base64-encoded string. The key
//     for this type will begin with `@bytes@`.
//   - AnyValue_ArrayValue: This OTLP type is stored as a JSON-encoded string.
//     The key for this type will begin with `@array@`.
//   - AnyValue_KVListValue: This OTLP type is stored as a JSON-encoded string.
//     The key for this type will begin with `@kvlist@`.
type SpanRow struct {
	// --- Span ---
	ID              string
	TraceID         string
	TraceState      string
	ParentSpanID    string
	Name            string
	Kind            string
	StartTime       time.Time
	StatusCode      string
	StatusMessage   string
	Duration        int64
	Attributes      Attributes
	EventNames      []string
	EventTimestamps []time.Time
	EventAttributes Attributes2D
	LinkTraceIDs    []string
	LinkSpanIDs     []string
	LinkTraceStates []string
	LinkAttributes  Attributes2D

	// --- Resource ---
	ServiceName string

	// --- Scope ---
	ScopeName    string
	ScopeVersion string
}

type Attributes struct {
	BoolKeys      []string
	BoolValues    []bool
	DoubleKeys    []string
	DoubleValues  []float64
	IntKeys       []string
	IntValues     []int64
	StrKeys       []string
	StrValues     []string
	ComplexKeys   []string
	ComplexValues []string
}

type Attributes2D struct {
	BoolKeys      [][]string
	BoolValues    [][]bool
	DoubleKeys    [][]string
	DoubleValues  [][]float64
	IntKeys       [][]string
	IntValues     [][]int64
	StrKeys       [][]string
	StrValues     [][]string
	ComplexKeys   [][]string
	ComplexValues [][]string
}

func ScanRow(rows driver.Rows) (*SpanRow, error) {
	var sr SpanRow
	err := rows.Scan(
		&sr.ID,
		&sr.TraceID,
		&sr.TraceState,
		&sr.ParentSpanID,
		&sr.Name,
		&sr.Kind,
		&sr.StartTime,
		&sr.StatusCode,
		&sr.StatusMessage,
		&sr.Duration,
		&sr.Attributes.BoolKeys,
		&sr.Attributes.BoolValues,
		&sr.Attributes.DoubleKeys,
		&sr.Attributes.DoubleValues,
		&sr.Attributes.IntKeys,
		&sr.Attributes.IntValues,
		&sr.Attributes.StrKeys,
		&sr.Attributes.StrValues,
		&sr.Attributes.ComplexKeys,
		&sr.Attributes.ComplexValues,
		&sr.EventNames,
		&sr.EventTimestamps,
		&sr.EventAttributes.BoolKeys,
		&sr.EventAttributes.BoolValues,
		&sr.EventAttributes.DoubleKeys,
		&sr.EventAttributes.DoubleValues,
		&sr.EventAttributes.IntKeys,
		&sr.EventAttributes.IntValues,
		&sr.EventAttributes.StrKeys,
		&sr.EventAttributes.StrValues,
		&sr.EventAttributes.ComplexKeys,
		&sr.EventAttributes.ComplexValues,
		&sr.LinkTraceIDs,
		&sr.LinkSpanIDs,
		&sr.LinkTraceStates,
		&sr.LinkAttributes.BoolKeys,
		&sr.LinkAttributes.BoolValues,
		&sr.LinkAttributes.DoubleKeys,
		&sr.LinkAttributes.DoubleValues,
		&sr.LinkAttributes.IntKeys,
		&sr.LinkAttributes.IntValues,
		&sr.LinkAttributes.StrKeys,
		&sr.LinkAttributes.StrValues,
		&sr.LinkAttributes.ComplexKeys,
		&sr.LinkAttributes.ComplexValues,
		&sr.ServiceName,
		&sr.ScopeName,
		&sr.ScopeVersion,
	)
	if err != nil {
		return nil, err
	}
	return &sr, nil
}
