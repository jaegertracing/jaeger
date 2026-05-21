// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"iter"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

// ServiceSummary holds per-service statistics for a single trace.
type ServiceSummary struct {
	Name           string
	SpanCount      int
	ErrorSpanCount int
}

// TraceSummary contains lightweight summary information about a trace,
// suitable for display in search result lists.
type TraceSummary struct {
	TraceID           pcommon.TraceID
	RootServiceName   string
	RootOperationName string
	StartTime         time.Time
	Duration          time.Duration
	SpanCount         int
	ErrorSpanCount    int
	// Services contains one entry per distinct service name observed across all spans.
	Services []ServiceSummary
}

// SummaryReader is an optional extension to tracestore.Reader that allows
// storage backends to compute trace summaries natively. Backends that do not
// implement this interface fall back to FindTraces + client-side aggregation.
//
// The iterator contract mirrors FindTraceIDs: each yielded batch may contain
// one or more summaries, and implementations may yield results incrementally
// as the underlying query executes rather than buffering all results first.
type SummaryReader interface {
	FindTraceSummaries(ctx context.Context, query TraceQueryParams) iter.Seq2[[]TraceSummary, error]
}
