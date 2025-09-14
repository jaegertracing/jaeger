// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package testdata

import (
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
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

var TraceID = pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})

var now = time.Date(2025, 6, 14, 10, 0, 0, 0, time.UTC)

var SingleSpan = []SpanRow{
	{
		ID:                          "0000000000000001",
		TraceID:                     TraceID.String(),
		TraceState:                  "state1",
		Name:                        "GET /api/user",
		Kind:                        "Server",
		StartTime:                   now,
		StatusCode:                  "Ok",
		StatusMessage:               "success",
		RawDuration:                 1_000_000_000,
		BoolAttributeKeys:           []string{"authenticated", "cache_hit"},
		BoolAttributeValues:         []bool{true, false},
		DoubleAttributeKeys:         []string{"response_time", "cpu_usage"},
		DoubleAttributeValues:       []float64{0.123, 45.67},
		IntAttributeKeys:            []string{"user_id", "request_size"},
		IntAttributeValues:          []int64{12345, 1024},
		StrAttributeKeys:            []string{"http.method", "http.url"},
		StrAttributeValues:          []string{"GET", "/api/user"},
		ComplexAttributeKeys:        []string{"@bytes@request_body"},
		ComplexAttributeValues:      []string{"eyJuYW1lIjoidGVzdCJ9"},
		EventNames:                  []string{"login"},
		EventTimestamps:             []time.Time{now},
		EventBoolAttributeKeys:      [][]string{{"event.authenticated", "event.cached"}},
		EventBoolAttributeValues:    [][]bool{{true, false}},
		EventDoubleAttributeKeys:    [][]string{{"event.response_time"}},
		EventDoubleAttributeValues:  [][]float64{{0.001}},
		EventIntAttributeKeys:       [][]string{{"event.sequence"}},
		EventIntAttributeValues:     [][]int64{{1}},
		EventStrAttributeKeys:       [][]string{{"event.message"}},
		EventStrAttributeValues:     [][]string{{"user login successful"}},
		EventComplexAttributeKeys:   [][]string{{"@bytes@event.payload"}},
		EventComplexAttributeValues: [][]string{{"eyJ1c2VyX2lkIjoxMjM0NX0="}},
		LinkTraceIDs:                []string{"00000000000000000000000000000002"},
		LinkSpanIDs:                 []string{"0000000000000002"},
		LinkTraceStates:             []string{"state2"},
		ServiceName:                 "user-service",
		ScopeName:                   "auth-scope",
		ScopeVersion:                "v1.0.0",
	},
}

var MultipleSpans = []SpanRow{
	{
		ID:                          "0000000000000001",
		TraceID:                     TraceID.String(),
		TraceState:                  "state1",
		Name:                        "GET /api/user",
		Kind:                        "Server",
		StartTime:                   now,
		StatusCode:                  "Ok",
		StatusMessage:               "success",
		RawDuration:                 1_000_000_000,
		BoolAttributeKeys:           []string{"authenticated", "cache_hit"},
		BoolAttributeValues:         []bool{true, false},
		DoubleAttributeKeys:         []string{"response_time", "cpu_usage"},
		DoubleAttributeValues:       []float64{0.123, 45.67},
		IntAttributeKeys:            []string{"user_id", "request_size"},
		IntAttributeValues:          []int64{12345, 1024},
		StrAttributeKeys:            []string{"http.method", "http.url"},
		StrAttributeValues:          []string{"GET", "/api/user"},
		ComplexAttributeKeys:        []string{"@bytes@request_body"},
		ComplexAttributeValues:      []string{"eyJuYW1lIjoidGVzdCJ9"},
		EventNames:                  []string{"login"},
		EventTimestamps:             []time.Time{now},
		EventBoolAttributeKeys:      [][]string{{"event.authenticated", "event.cached"}},
		EventBoolAttributeValues:    [][]bool{{true, false}},
		EventDoubleAttributeKeys:    [][]string{{"event.response_time"}},
		EventDoubleAttributeValues:  [][]float64{{0.001}},
		EventIntAttributeKeys:       [][]string{{"event.sequence"}},
		EventIntAttributeValues:     [][]int64{{1}},
		EventStrAttributeKeys:       [][]string{{"event.message"}},
		EventStrAttributeValues:     [][]string{{"user login successful"}},
		EventComplexAttributeKeys:   [][]string{{"@bytes@event.payload"}},
		EventComplexAttributeValues: [][]string{{"eyJ1c2VyX2lkIjoxMjM0NX0="}},
		LinkTraceIDs:                []string{"00000000000000000000000000000002"},
		LinkSpanIDs:                 []string{"0000000000000002"},
		LinkTraceStates:             []string{"state2"},
		ServiceName:                 "user-service",
		ScopeName:                   "auth-scope",
		ScopeVersion:                "v1.0.0",
	},
	{
		ID:                          "0000000000000003",
		TraceID:                     TraceID.String(),
		TraceState:                  "state1",
		ParentSpanID:                "0000000000000001",
		Name:                        "SELECT /db/query",
		Kind:                        "Client",
		StartTime:                   now.Add(10 * time.Millisecond),
		StatusCode:                  "Ok",
		StatusMessage:               "success",
		RawDuration:                 500_000_000,
		BoolAttributeKeys:           []string{"db.cached", "db.readonly"},
		BoolAttributeValues:         []bool{false, true},
		DoubleAttributeKeys:         []string{"db.latency", "db.connections"},
		DoubleAttributeValues:       []float64{0.05, 5.0},
		IntAttributeKeys:            []string{"db.rows_affected", "db.connection_id"},
		IntAttributeValues:          []int64{150, 42},
		StrAttributeKeys:            []string{"db.statement", "db.name"},
		StrAttributeValues:          []string{"SELECT * FROM users", "userdb"},
		ComplexAttributeKeys:        []string{"@bytes@db.query_plan"},
		ComplexAttributeValues:      []string{"UExBTiBTRUxFQ1Q="},
		EventNames:                  []string{"query-start", "query-end"},
		EventTimestamps:             []time.Time{now.Add(10 * time.Millisecond), now.Add(510 * time.Millisecond)},
		EventBoolAttributeKeys:      [][]string{{"db.optimized", "db.indexed"}, {"db.cached", "db.successful"}},
		EventBoolAttributeValues:    [][]bool{{true, false}, {true, false}},
		EventDoubleAttributeKeys:    [][]string{{"db.query_time"}, {"db.result_time"}},
		EventDoubleAttributeValues:  [][]float64{{0.001}, {0.5}},
		EventIntAttributeKeys:       [][]string{{"db.connection_pool_size"}, {"db.result_count"}},
		EventIntAttributeValues:     [][]int64{{10}, {150}},
		EventStrAttributeKeys:       [][]string{{"db.event.type"}, {"db.event.status"}},
		EventStrAttributeValues:     [][]string{{"query_execution_start"}, {"query_execution_complete"}},
		EventComplexAttributeKeys:   [][]string{{"@bytes@db.query_metadata"}, {"@bytes@db.result_metadata"}},
		EventComplexAttributeValues: [][]string{{"eyJxdWVyeV9pZCI6MTIzfQ=="}, {"eyJyb3dfY291bnQiOjE1MH0="}},
		LinkTraceIDs:                []string{},
		LinkSpanIDs:                 []string{},
		LinkTraceStates:             []string{},
		ServiceName:                 "db-service",
		ScopeName:                   "db-scope",
		ScopeVersion:                "v1.0.0",
	},
}
