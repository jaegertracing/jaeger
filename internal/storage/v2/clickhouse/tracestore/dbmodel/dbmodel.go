// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import "time"

// Span represents a single row in the ClickHouse `spans` table.
type Span struct {
	// --- Span ---
	Id            string        `ch:"id"`
	TraceID       string        `ch:"trace_id"`
	TraceState    string        `ch:"trace_state"`
	ParentSpanID  string        `ch:"parent_span_id"`
	Name          string        `ch:"name"`
	Kind          string        `ch:"kind"`
	StartTime     time.Time     `ch:"start_time"`
	Duration      time.Duration `ch:"duration"`
	Events        []Event
	Links         []Link
	StatusCode    string `ch:"status_code"`
	StatusMessage string `ch:"status_message"`

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
	TraceId    string
	SpanId     string
	TraceState string
}

type Event struct {
	Name       string
	Timestamp  time.Time
	Attributes AttributesGroup
}

type Service struct {
	Name string `ch:"name"`
}

type Operation struct {
	Name     string `ch:"name"`
	SpanKind string `ch:"span_kind"`
}
