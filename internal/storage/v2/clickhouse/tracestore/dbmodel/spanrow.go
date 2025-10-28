// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// SpanRow represents a single record in the ClickHouse `spans` table.
//
// Complex attributes are non-primitive OTLP types that require special serialization
// before being stored. These types are encoded as follows:
//
//   - pcommon.ValueTypeBytes:
//     Represents raw byte data. The value is Base64-encoded and stored as a string.
//     Keys for this type are prefixed with `@bytes@`.
//
//   - pcommon.ValueTypeSlice:
//     Represents an OTLP slice (array). The value is first serialized to JSON, then
//     Base64-encoded before storage. Keys for this type are prefixed with `@slice@`.
//
//   - pcommon.ValueTypeMap:
//     Represents an OTLP map. The value is first serialized to JSON, then
//     Base64-encoded before storage. Keys for this type are prefixed with `@map@`.
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
	ServiceName        string
	ResourceAttributes Attributes

	// --- Scope ---
	ScopeName       string
	ScopeVersion    string
	ScopeAttributes Attributes
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
		&sr.ResourceAttributes.BoolKeys,
		&sr.ResourceAttributes.BoolValues,
		&sr.ResourceAttributes.DoubleKeys,
		&sr.ResourceAttributes.DoubleValues,
		&sr.ResourceAttributes.IntKeys,
		&sr.ResourceAttributes.IntValues,
		&sr.ResourceAttributes.StrKeys,
		&sr.ResourceAttributes.StrValues,
		&sr.ResourceAttributes.ComplexKeys,
		&sr.ResourceAttributes.ComplexValues,
		&sr.ScopeName,
		&sr.ScopeVersion,
		&sr.ScopeAttributes.BoolKeys,
		&sr.ScopeAttributes.BoolValues,
		&sr.ScopeAttributes.DoubleKeys,
		&sr.ScopeAttributes.DoubleValues,
		&sr.ScopeAttributes.IntKeys,
		&sr.ScopeAttributes.IntValues,
		&sr.ScopeAttributes.StrKeys,
		&sr.ScopeAttributes.StrValues,
		&sr.ScopeAttributes.ComplexKeys,
		&sr.ScopeAttributes.ComplexValues,
	)
	if err != nil {
		return nil, err
	}
	return &sr, nil
}
