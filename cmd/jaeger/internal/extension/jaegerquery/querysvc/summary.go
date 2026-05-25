// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"cmp"
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
		for traces, err := range jptrace.AggregateTraces(seq) {
			if err != nil {
				yield(nil, err)
				return
			}
			adj.Adjust(traces)
			if !yield([]tracestore.TraceSummary{summarizeTrace(traces)}, nil) {
				return
			}
		}
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
		rootStartTime     time.Time
		minStartTime      time.Time
		maxEndTime        time.Time
		totalSpans        int
		totalErrors       int
	)

	// First pass: collect all span IDs present in the trace.
	for _, span := range jptrace.SpanIter(traces) {
		spanIDs[span.SpanID()] = struct{}{}
	}

	// Second pass: compute statistics.
	var orphanSpans int
	for pos, span := range jptrace.SpanIter(traces) {
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
		if minStartTime.IsZero() || spanStart.Before(minStartTime) {
			minStartTime = spanStart
		}
		if maxEndTime.IsZero() || spanEnd.After(maxEndTime) {
			maxEndTime = spanEnd
		}

		parentID := span.ParentSpanID()
		if parentID.IsEmpty() {
			// Among spans with no parent, pick the one with the earliest start time.
			if rootStartTime.IsZero() || spanStart.Before(rootStartTime) {
				rootServiceName = svcName
				rootOperationName = span.Name()
				rootStartTime = spanStart
			}
		} else if _, ok := spanIDs[parentID]; !ok {
			orphanSpans++
		}
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
		return cmp.Compare(a.Name, b.Name)
	})

	return tracestore.TraceSummary{
		TraceID:           jptrace.GetTraceID(traces),
		RootServiceName:   rootServiceName,
		RootOperationName: rootOperationName,
		MinStartTime:      minStartTime,
		MaxEndTime:        maxEndTime,
		SpanCount:         totalSpans,
		ErrorSpanCount:    totalErrors,
		OrphanSpanCount:   orphanSpans,
		Services:          svcSummaries,
	}
}
