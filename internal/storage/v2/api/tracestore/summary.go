// Copyright (c) 2026 The Jaeger Authors.
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
	TraceID pcommon.TraceID
	// RootServiceName is the service name of the root span (the span with no parent
	// span ID). If multiple spans have no parent, the one with the earliest start
	// timestamp is chosen as the root.
	RootServiceName string
	// RootOperationName is the operation name of the root span (see RootServiceName).
	RootOperationName string
	// MinStartTime is the start timestamp of the earliest span in the trace.
	MinStartTime time.Time
	// MaxEndTime is the end timestamp across all spans in the trace
	// (i.e. max of all span end timestamps). Duration can be derived as
	// MaxEndTime - MinStartTime.
	MaxEndTime     time.Time
	SpanCount      int
	ErrorSpanCount int
	// OrphanSpanCount is the number of spans that have a parent span ID that
	// is not present in this trace (i.e. the trace is incomplete).
	OrphanSpanCount int
	// Services contains one entry per distinct service name observed across all spans,
	// including the root span's service. Entries are sorted by service name.
	Services []ServiceSummary
}

// SummaryReader is an optional extension to tracestore.Reader that allows
// storage backends to compute trace summaries natively. Backends that do not
// implement this interface fall back to FindTraces + client-side aggregation.
//
// The iterator streams result batches. Implementations that do not support the
// operation should yield errors.ErrUnsupported (wrapped with %w) as the first
// error; the caller will fall back to FindTraces + client-side aggregation.
// Each yielded batch may contain one or more summaries; implementations may
// yield incrementally rather than buffering all results first.
type SummaryReader interface {
	FindTraceSummaries(ctx context.Context, query TraceQueryParams) iter.Seq2[[]TraceSummary, error]
}

// AsSummaryReader returns r as a SummaryReader if it implements the interface,
// either directly or through a chain of Unwrap() methods (decorators such as
// tracestoremetrics.ReadMetricsDecorator expose the wrapped reader via Unwrap
// rather than forwarding SummaryReader). It returns nil when no SummaryReader is
// found in the chain.
func AsSummaryReader(r Reader) SummaryReader {
	type unwrapper interface {
		Unwrap() Reader
	}
	for r != nil {
		if sr, ok := r.(SummaryReader); ok {
			return sr
		}
		u, ok := r.(unwrapper)
		if !ok {
			return nil
		}
		r = u.Unwrap()
	}
	return nil
}
