// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

// TODO: the JSON types below are temporary scaffolding until the FindTraceSummaries
// RPC is added to jaeger-idl and proto-generated types replace them (ADR-010, Milestone 3).

// serviceSummaryJSON is the JSON representation of a per-service summary.
type serviceSummaryJSON struct {
	Name           string `json:"name"`
	SpanCount      int    `json:"spanCount"`
	ErrorSpanCount int    `json:"errorSpanCount"`
}

// traceSummaryJSON is the JSON representation of a trace summary.
// Timestamps are Unix nanoseconds encoded as decimal strings, consistent with
// OTLP proto3 JSON encoding (e.g. startTimeUnixNano in the OTLP span JSON
// returned by GET /api/v3/traces). String encoding avoids float64 precision
// loss in JavaScript for nanosecond values above 2^53.
type traceSummaryJSON struct {
	TraceID              string               `json:"traceId"`
	RootServiceName      string               `json:"rootServiceName"`
	RootOperationName    string               `json:"rootOperationName"`
	MinStartTimeUnixNano string               `json:"minStartTimeUnixNano,omitempty"`
	MaxEndTimeUnixNano   string               `json:"maxEndTimeUnixNano,omitempty"`
	SpanCount            int                  `json:"spanCount"`
	ErrorSpanCount       int                  `json:"errorSpanCount"`
	OrphanSpanCount      int                  `json:"orphanSpanCount"`
	Services             []serviceSummaryJSON `json:"services"`
}

// findTraceSummariesResponseJSON is the JSON envelope for the FindTraceSummaries response.
type findTraceSummariesResponseJSON struct {
	Summaries []traceSummaryJSON `json:"summaries"`
}
