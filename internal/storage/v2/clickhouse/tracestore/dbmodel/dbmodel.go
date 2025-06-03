// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"time"
)

// Span represents a single row in the ClickHouse `spans` table.
type Span struct {
	// --- Span ---
	// TODO: add attributes
	ID            string    `ch:"id"`
	TraceID       string    `ch:"trace_id"`
	TraceState    string    `ch:"trace_state"`
	ParentSpanID  string    `ch:"parent_span_id"`
	Name          string    `ch:"name"`
	Kind          string    `ch:"kind"`
	StartTime     time.Time `ch:"start_time"`
	StatusCode    string    `ch:"status_code"`
	StatusMessage string    `ch:"status_message"`

	// Duration is stored in ClickHouse as a UInt64 representing the number of nanoseconds.
	// In Go, it is manually converted to and from time.Duration for convenience.
	Duration time.Duration

	// --- Nested Types ---
	// The fields below correspond to ClickHouse Nested columns, which act
	// like a table inside a cell. The clickhouse-go driver does not support
	// automatic decoding of Nested types via ScanStruct into these slice
	// structs directly. Therefore, the raw data for these fields must be
	// scanned into intermediate types (e.g., []map[string]any), and then
	// manually decoded into the concrete Go structs defined here.
	// For this reason, these fields do NOT have `ch` tags themselves.
	// (Ref: https://github.com/ClickHouse/clickhouse-go/blob/main/examples/clickhouse_api/nested.go)
	Events []Event
	Links  []Link

	// --- Resource ---
	// TODO: add attributes
	ServiceName string `ch:"service_name"`

	// --- Scope ---
	// TODO: add attributes
	ScopeName    string `ch:"scope_name"`
	ScopeVersion string `ch:"scope_version"`
}

type Link struct {
	// TODO: add attributes
	TraceID    string
	SpanID     string
	TraceState string
}

func getLinksFromRaw(raw []map[string]any) []Link {
	links := make([]Link, 0, len(raw))
	for _, m := range raw {
		links = append(links, getLinkFromRaw(m))
	}
	return links
}

func getLinkFromRaw(m map[string]any) Link {
	var link Link
	if traceID, ok := m["trace_id"].(string); ok {
		link.TraceID = traceID
	}
	if spanID, ok := m["span_id"].(string); ok {
		link.SpanID = spanID
	}
	if traceState, ok := m["trace_state"].(string); ok {
		link.TraceState = traceState
	}
	return link
}

type Event struct {
	// TODO: add attributes
	Name      string
	Timestamp time.Time
}

func getEventsFromRaw(raw []map[string]any) []Event {
	events := make([]Event, 0, len(raw))
	for _, m := range raw {
		events = append(events, getEventFromRaw(m))
	}
	return events
}

func getEventFromRaw(m map[string]any) Event {
	var event Event
	if name, ok := m["name"].(string); ok {
		event.Name = name
	}
	if ts, ok := m["timestamp"].(time.Time); ok {
		event.Timestamp = ts
	}
	return event
}

type Service struct {
	Name string `ch:"name"`
}

type Operation struct {
	Name     string `ch:"name"`
	SpanKind string `ch:"span_kind"`
}
