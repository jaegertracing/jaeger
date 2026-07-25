// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"
	"fmt"
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

// UnsupportedTraceSummaries provides a Reader.FindTraceSummaries implementation
// for backends that cannot compute trace summaries natively. It yields
// errors.ErrUnsupported as its first (and only) error, which signals the caller
// to fall back to FindTraces + client-side aggregation. Embed it in a Reader to
// opt into that behavior without writing the method by hand.
type UnsupportedTraceSummaries struct{}

func (UnsupportedTraceSummaries) FindTraceSummaries(context.Context, TraceQueryParams) iter.Seq2[[]TraceSummary, error] {
	return func(yield func([]TraceSummary, error) bool) {
		yield(nil, fmt.Errorf("this storage backend does not compute trace summaries natively: %w", errors.ErrUnsupported))
	}
}
