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
	ID                          string
	TraceID                     string
	TraceState                  string
	ParentSpanID                string
	Name                        string
	Kind                        string
	StartTime                   time.Time
	StatusCode                  string
	StatusMessage               string
	RawDuration                 int64
	BoolAttributeKeys           []string
	BoolAttributeValues         []bool
	DoubleAttributeKeys         []string
	DoubleAttributeValues       []float64
	IntAttributeKeys            []string
	IntAttributeValues          []int64
	StrAttributeKeys            []string
	StrAttributeValues          []string
	ComplexAttributeKeys        []string
	ComplexAttributeValues      []string
	EventNames                  []string
	EventTimestamps             []time.Time
	EventBoolAttributeKeys      [][]string
	EventBoolAttributeValues    [][]bool
	EventDoubleAttributeKeys    [][]string
	EventDoubleAttributeValues  [][]float64
	EventIntAttributeKeys       [][]string
	EventIntAttributeValues     [][]int64
	EventStrAttributeKeys       [][]string
	EventStrAttributeValues     [][]string
	EventComplexAttributeKeys   [][]string
	EventComplexAttributeValues [][]string
	LinkTraceIDs                []string
	LinkSpanIDs                 []string
	LinkTraceStates             []string
	LinkBoolAttributeKeys       [][]string
	LinkBoolAttributeValues     [][]bool
	LinkDoubleAttributeKeys     [][]string
	LinkDoubleAttributeValues   [][]float64
	LinkIntAttributeKeys        [][]string
	LinkIntAttributeValues      [][]int64
	LinkStrAttributeKeys        [][]string
	LinkStrAttributeValues      [][]string
	LinkComplexAttributeKeys    [][]string
	LinkComplexAttributeValues  [][]string

	// --- Resource ---
	ServiceName string

	// --- Scope ---
	ScopeName    string
	ScopeVersion string
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
		&sr.RawDuration,
		&sr.BoolAttributeKeys,
		&sr.BoolAttributeValues,
		&sr.DoubleAttributeKeys,
		&sr.DoubleAttributeValues,
		&sr.IntAttributeKeys,
		&sr.IntAttributeValues,
		&sr.StrAttributeKeys,
		&sr.StrAttributeValues,
		&sr.ComplexAttributeKeys,
		&sr.ComplexAttributeValues,
		&sr.EventNames,
		&sr.EventTimestamps,
		&sr.EventBoolAttributeKeys,
		&sr.EventBoolAttributeValues,
		&sr.EventDoubleAttributeKeys,
		&sr.EventDoubleAttributeValues,
		&sr.EventIntAttributeKeys,
		&sr.EventIntAttributeValues,
		&sr.EventStrAttributeKeys,
		&sr.EventStrAttributeValues,
		&sr.EventComplexAttributeKeys,
		&sr.EventComplexAttributeValues,
		&sr.LinkTraceIDs,
		&sr.LinkSpanIDs,
		&sr.LinkTraceStates,
		&sr.LinkBoolAttributeKeys,
		&sr.LinkBoolAttributeValues,
		&sr.LinkDoubleAttributeKeys,
		&sr.LinkDoubleAttributeValues,
		&sr.LinkIntAttributeKeys,
		&sr.LinkIntAttributeValues,
		&sr.LinkStrAttributeKeys,
		&sr.LinkStrAttributeValues,
		&sr.LinkComplexAttributeKeys,
		&sr.LinkComplexAttributeValues,
		&sr.ServiceName,
		&sr.ScopeName,
		&sr.ScopeVersion,
	)
	if err != nil {
		return nil, err
	}
	return &sr, nil
}
