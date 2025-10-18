// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type SpanRow struct {
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
	ServiceName                 string
	ScopeName                   string
	ScopeVersion                string
}

func ScanRow(rows driver.Rows) (*SpanRow, error) {
	var sr *SpanRow
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
		&sr.ServiceName,
		&sr.ScopeName,
		&sr.ScopeVersion,
	)
	if err != nil {
		return nil, err
	}
	return sr, nil
}

// // Span represents a single row in the ClickHouse `spans` table.
// type Span struct {
// 	// --- Span ---
// 	ID            string
// 	TraceID       string
// 	TraceState    string
// 	ParentSpanID  string
// 	Name          string
// 	Kind          string
// 	StartTime     time.Time
// 	StatusCode    string
// 	StatusMessage string

// 	// Duration is stored in ClickHouse as a UInt64 representing the number of nanoseconds.
// 	// In Go, it is manually converted to and from time.Duration for convenience.
// 	Duration time.Duration

// 	// --- Nested Types ---
// 	// The fields below correspond to ClickHouse Nested columns, which act
// 	// like a table inside a cell. The clickhouse-go driver does not support
// 	// automatic decoding of Nested types via ScanStruct into these slice
// 	// structs directly. Therefore, the raw data for these fields must be
// 	// scanned into intermediate types (e.g., []map[string]any), and then
// 	// manually decoded into the concrete Go structs defined here.
// 	// For this reason, these fields do NOT have `ch` tags themselves.
// 	// (Ref: https://github.com/ClickHouse/clickhouse-go/blob/main/examples/clickhouse_api/nested.go)
// 	Events []Event
// 	Links  []Link

// 	Attributes Attributes

// 	// --- Resource ---
// 	// TODO: add attributes
// 	ServiceName string

// 	// --- Scope ---
// 	// TODO: add attributes
// 	ScopeName    string
// 	ScopeVersion string
// }

// type Attributes struct {
// 	BoolAttributes   []Attribute[bool]
// 	DoubleAttributes []Attribute[float64]
// 	IntAttributes    []Attribute[int64]
// 	StrAttributes    []Attribute[string]
// 	// ComplexAttributes are attributes that are not of a primitive type and hence need special handling.
// 	// The following OTLP types are stored here:
// 	// - AnyValue_BytesValue: This OTLP type is stored as a base64-encoded string. The key
// 	// 	for this type will begin with `@bytes@`.
// 	// - AnyValue_ArrayValue: This OTLP type is stored as a JSON-encoded string.
// 	// 	The key for this type will begin with `@array@`.
// 	// - AnyValue_KVListValue: This OTLP type is stored as a JSON-encoded string.
// 	// 	The key for this type will begin with `@kvlist@`.
// 	ComplexAttributes []Attribute[string]
// }

// type Attribute[T any] struct {
// 	Key   string
// 	Value T
// }

// type Link struct {
// 	// TODO: add attributes
// 	TraceID    string
// 	SpanID     string
// 	TraceState string
// }

// func getLinksFromRaw(raw []map[string]any) []Link {
// 	links := make([]Link, 0, len(raw))
// 	for _, m := range raw {
// 		links = append(links, getLinkFromRaw(m))
// 	}
// 	return links
// }

// func getLinkFromRaw(m map[string]any) Link {
// 	var link Link
// 	if traceID, ok := m["trace_id"].(string); ok {
// 		link.TraceID = traceID
// 	}
// 	if spanID, ok := m["span_id"].(string); ok {
// 		link.SpanID = spanID
// 	}
// 	if traceState, ok := m["trace_state"].(string); ok {
// 		link.TraceState = traceState
// 	}
// 	return link
// }

// type Event struct {
// 	Name       string
// 	Timestamp  time.Time
// 	Attributes Attributes
// }

// func getEventsFromRaw(raw []map[string]any) []Event {
// 	events := make([]Event, 0, len(raw))
// 	for _, m := range raw {
// 		events = append(events, getEventFromRaw(m))
// 	}
// 	return events
// }

// func getEventFromRaw(m map[string]any) Event {
// 	var event Event
// 	if name, ok := m["name"].(string); ok {
// 		event.Name = name
// 	}
// 	if ts, ok := m["timestamp"].(time.Time); ok {
// 		event.Timestamp = ts
// 	}
// 	return event
// }

// type Service struct {
// 	Name string `ch:"name"`
// }

// type Operation struct {
// 	Name     string `ch:"name"`
// 	SpanKind string `ch:"span_kind"`
// }
