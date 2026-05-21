// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"iter"
	"slices"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/adjuster"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

func computeSummaries(seq iter.Seq2[[]ptrace.Traces, error], adj adjuster.Adjuster) iter.Seq2[[]tracestore.TraceSummary, error] {
	return func(yield func([]tracestore.TraceSummary, error) bool) {
		jptrace.AggregateTraces(seq)(func(traces ptrace.Traces, err error) bool {
			if err != nil {
				yield(nil, err)
				return false
			}
			adj.Adjust(traces)
			return yield([]tracestore.TraceSummary{summarizeTrace(traces)}, nil)
		})
	}
}

func summarizeTrace(traces ptrace.Traces) tracestore.TraceSummary {
	type perSvcStats struct {
		spanCount      int
		errorSpanCount int
	}
	services := make(map[string]*perSvcStats)
	spanIDs := make(map[pcommon.SpanID]struct{})

	var (
		rootServiceName   string
		rootOperationName string
		rootFound         bool
		startTime         time.Time
		endTime           time.Time
		totalSpans        int
		totalErrors       int
	)

	// First pass: collect all span IDs present in the trace.
	jptrace.SpanIter(traces)(func(_ jptrace.SpanIterPos, span ptrace.Span) bool {
		spanIDs[span.SpanID()] = struct{}{}
		return true
	})

	// Second pass: compute statistics.
	var orphanSpans int
	jptrace.SpanIter(traces)(func(pos jptrace.SpanIterPos, span ptrace.Span) bool {
		svcName := ""
		if v, ok := pos.Resource.Resource().Attributes().Get(otelsemconv.ServiceNameKey); ok {
			svcName = v.Str()
		}

		svcStats := services[svcName]
		if svcStats == nil {
			svcStats = &perSvcStats{}
			services[svcName] = svcStats
		}
		svcStats.spanCount++
		totalSpans++

		if span.Status().Code() == ptrace.StatusCodeError {
			svcStats.errorSpanCount++
			totalErrors++
		}

		spanStart := span.StartTimestamp().AsTime()
		spanEnd := span.EndTimestamp().AsTime()
		if startTime.IsZero() || spanStart.Before(startTime) {
			startTime = spanStart
		}
		if endTime.IsZero() || spanEnd.After(endTime) {
			endTime = spanEnd
		}

		parentID := span.ParentSpanID()
		if parentID.IsEmpty() {
			if !rootFound {
				rootServiceName = svcName
				rootOperationName = span.Name()
				rootFound = true
			}
		} else if _, ok := spanIDs[parentID]; !ok {
			orphanSpans++
		}
		return true
	})

	var duration time.Duration
	if !startTime.IsZero() {
		duration = endTime.Sub(startTime)
	}

	svcSummaries := make([]tracestore.ServiceSummary, 0, len(services))
	for name, svcStats := range services {
		svcSummaries = append(svcSummaries, tracestore.ServiceSummary{
			Name:           name,
			SpanCount:      svcStats.spanCount,
			ErrorSpanCount: svcStats.errorSpanCount,
		})
	}
	slices.SortFunc(svcSummaries, func(a, b tracestore.ServiceSummary) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})

	return tracestore.TraceSummary{
		TraceID:           jptrace.GetTraceID(traces),
		RootServiceName:   rootServiceName,
		RootOperationName: rootOperationName,
		StartTime:         startTime,
		Duration:          duration,
		SpanCount:         totalSpans,
		ErrorSpanCount:    totalErrors,
		OrphanSpanCount:   orphanSpans,
		Services:          svcSummaries,
	}
}
