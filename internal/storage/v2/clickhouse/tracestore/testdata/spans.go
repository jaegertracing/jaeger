// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package testdata

import (
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

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

var TraceID = pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})

var now = time.Date(2025, 6, 14, 10, 0, 0, 0, time.UTC)

var SingleSpan = []SpanRow{
	{
		ID:              "0000000000000001",
		TraceID:         TraceID.String(),
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
	},
}

var MultipleSpans = []SpanRow{
	{
		ID:              "0000000000000001",
		TraceID:         TraceID.String(),
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
	},
	{
		ID:              "0000000000000003",
		TraceID:         TraceID.String(),
		TraceState:      "state1",
		ParentSpanID:    "0000000000000001",
		Name:            "SELECT /db/query",
		Kind:            "Client",
		StartTime:       now.Add(10 * time.Millisecond),
		StatusCode:      "Ok",
		StatusMessage:   "success",
		RawDuration:     500_000_000,
		EventNames:      []string{"query-start", "query-end"},
		EventTimestamps: []time.Time{now.Add(10 * time.Millisecond), now.Add(510 * time.Millisecond)},
		LinkTraceIDs:    []string{},
		LinkSpanIDs:     []string{},
		LinkTraceStates: []string{},
		ServiceName:     "db-service",
		ScopeName:       "db-scope",
		ScopeVersion:    "v1.0.0",
	},
}
