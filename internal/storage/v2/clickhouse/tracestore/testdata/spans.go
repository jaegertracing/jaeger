// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package testdata

import "time"

type SpanRow struct {
	ID              string
	TraceID         string
	TraceState      string
	ParentSpanID    string
	Name            string
	Kind            string
	StartTime       time.Time
	StatusCode      string
	StatusMessage   string
	RawDuration     int64
	EventNames      []string
	EventTimestamps []time.Time
	LinkTraceIDs    []string
	LinkSpanIDs     []string
	LinkTraceStates []string
	ServiceName     string
	ScopeName       string
	ScopeVersion    string
}

var now = time.Date(2025, 6, 14, 10, 0, 0, 0, time.UTC)

var SingleSpan = SpanRow{
	ID:              "0000000000000001",
	TraceID:         "00000000000000000000000000000001",
	TraceState:      "state1",
	Name:            "GET /api/user",
	Kind:            "Server",
	StartTime:       now,
	StatusCode:      "Ok",
	StatusMessage:   "success",
	RawDuration:     1_000_000_000,
	EventNames:      []string{"login"},
	EventTimestamps: []time.Time{now},
	LinkTraceIDs:    []string{"00000000000000000000000000000002"},
	LinkSpanIDs:     []string{"0000000000000002"},
	LinkTraceStates: []string{"state2"},
	ServiceName:     "user-service",
	ScopeName:       "auth-scope",
	ScopeVersion:    "v1.0.0",
}
