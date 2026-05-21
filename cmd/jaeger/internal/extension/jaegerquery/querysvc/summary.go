// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"context"
	"iter"
	"time"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

// FindTraceSummaries searches for traces matching the query and returns an iterator
// of lightweight summary information. If the underlying storage implements
// tracestore.SummaryReader, it delegates to that; otherwise it falls back to
// FindTraces and computes summaries from the full trace data.
// The iterator is single-use: once consumed, it cannot be used again.
func (qs QueryService) FindTraceSummaries(
	ctx context.Context,
	query TraceQueryParams,
) iter.Seq2[[]tracestore.TraceSummary, error] {
	if sr, ok := qs.traceReader.(tracestore.SummaryReader); ok {
		return sr.FindTraceSummaries(ctx, query.TraceQueryParams)
	}
	return computeSummaries(qs.traceReader.FindTraces(ctx, query.TraceQueryParams))
}

func computeSummaries(seq iter.Seq2[[]ptrace.Traces, error]) iter.Seq2[[]tracestore.TraceSummary, error] {
	return func(yield func([]tracestore.TraceSummary, error) bool) {
		seq(func(batch []ptrace.Traces, err error) bool {
			if err != nil {
				yield(nil, err)
				return false
			}
			summaries := make([]tracestore.TraceSummary, len(batch))
			for i, traces := range batch {
				summaries[i] = summarizeTrace(traces)
			}
			return yield(summaries, nil)
		})
	}
}

func summarizeTrace(traces ptrace.Traces) tracestore.TraceSummary {
	type svcStats struct {
		spanCount      int
		errorSpanCount int
	}
	services := make(map[string]*svcStats)

	var (
		rootServiceName   string
		rootOperationName string
		rootFound         bool
		startTime         time.Time
		endTime           time.Time
		totalSpans        int
		totalErrors       int
	)

	jptrace.SpanIter(traces)(func(pos jptrace.SpanIterPos, span ptrace.Span) bool {
		svcName := ""
		if v, ok := pos.Resource.Resource().Attributes().Get(otelsemconv.ServiceNameKey); ok {
			svcName = v.Str()
		}

		stats := services[svcName]
		if stats == nil {
			stats = &svcStats{}
			services[svcName] = stats
		}
		stats.spanCount++
		totalSpans++

		if span.Status().Code() == ptrace.StatusCodeError {
			stats.errorSpanCount++
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

		if !rootFound && span.ParentSpanID().IsEmpty() {
			rootServiceName = svcName
			rootOperationName = span.Name()
			rootFound = true
		}
		return true
	})

	var duration time.Duration
	if !startTime.IsZero() {
		duration = endTime.Sub(startTime)
	}

	svcSummaries := make([]tracestore.ServiceSummary, 0, len(services))
	for name, stats := range services {
		svcSummaries = append(svcSummaries, tracestore.ServiceSummary{
			Name:           name,
			SpanCount:      stats.spanCount,
			ErrorSpanCount: stats.errorSpanCount,
		})
	}

	return tracestore.TraceSummary{
		TraceID:           jptrace.GetTraceID(traces),
		RootServiceName:   rootServiceName,
		RootOperationName: rootOperationName,
		StartTime:         startTime,
		Duration:          duration,
		SpanCount:         totalSpans,
		ErrorSpanCount:    totalErrors,
		Services:          svcSummaries,
	}
}
