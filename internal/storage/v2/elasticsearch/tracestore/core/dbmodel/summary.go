// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

// ServiceSummary holds per-service statistics for a single trace.
type ServiceSummary struct {
	ServiceName    string
	SpanCount      int
	ErrorSpanCount int
}

// TraceSummary is the DB-level trace summary computed by ElasticSearch
// aggregations. The wrapping TraceReader converts it to tracestore.TraceSummary.
type TraceSummary struct {
	TraceID           TraceID
	RootServiceName   string
	RootOperationName string
	// MinStartTime and MaxEndTime are microseconds since the Unix epoch.
	MinStartTime   uint64
	MaxEndTime     uint64
	SpanCount      int
	ErrorSpanCount int
	Services       []ServiceSummary
}
