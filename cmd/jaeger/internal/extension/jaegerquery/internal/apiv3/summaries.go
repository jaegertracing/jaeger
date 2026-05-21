// Copyright (c) 2025 The Jaeger Authors.
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
type traceSummaryJSON struct {
	TraceID           string               `json:"traceID"`
	RootServiceName   string               `json:"rootServiceName"`
	RootOperationName string               `json:"rootOperationName"`
	StartTimeUnixUs   int64                `json:"startTimeUnixUs,omitempty"`
	DurationUs        int64                `json:"durationUs"`
	SpanCount         int                  `json:"spanCount"`
	ErrorSpanCount    int                  `json:"errorSpanCount"`
	OrphanSpanCount   int                  `json:"orphanSpanCount"`
	Services          []serviceSummaryJSON `json:"services"`
}

// findTraceSummariesResponseJSON is the JSON envelope for the FindTraceSummaries response.
type findTraceSummariesResponseJSON struct {
	Summaries []traceSummaryJSON `json:"summaries"`
}
