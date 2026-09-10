// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"iter"
	"time"

	"go.opentelemetry.io/collector/featuregate"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

// nativeTraceSummariesGate enables computing trace summaries (the metadata shown
// on the search-results page) natively in Elasticsearch/OpenSearch via a single
// aggregation query, instead of loading full traces and aggregating them in the
// query service. Enabled by default; when disabled, FindTraceSummaries yields
// errors.ErrUnsupported, so the query service transparently falls back to the
// full-trace path.
var nativeTraceSummariesGate = featuregate.GlobalRegistry().MustRegister(
	"jaeger.es.nativeTraceSummaries",
	featuregate.StageBeta,
	featuregate.WithRegisterFromVersion("v2.20.0"),
	featuregate.WithRegisterDescription(
		"Computes trace summaries natively in Elasticsearch/OpenSearch via aggregations "+
			"instead of loading full traces and aggregating in the query service. Requires "+
			"inline (Painless) scripts to be enabled on the cluster.",
	),
)

// FindTraceSummaries computes trace summaries via a storage-side aggregation when the
// native-summaries feature gate is enabled. When it is disabled, or when the backend
// cannot compute them (e.g. Painless scripting is disabled, which the core reader
// surfaces as errors.ErrUnsupported), it yields errors.ErrUnsupported so the query
// service falls back to loading full traces and aggregating client-side.
func (r *TraceReader) FindTraceSummaries(ctx context.Context, query tracestore.TraceQueryParams) iter.Seq2[[]tracestore.TraceSummary, error] {
	if !nativeTraceSummariesGate.IsEnabled() {
		return tracestore.UnsupportedTraceSummaries{}.FindTraceSummaries(ctx, query)
	}
	return func(yield func([]tracestore.TraceSummary, error) bool) {
		// The aggregation returns all matching summaries in a single ES response,
		// so they are materialized and yielded in one batch (allowed by the
		// FindTraceSummaries contract).
		dbSummaries, err := r.spanReader.FindTraceSummaries(ctx, toDBTraceQueryParams(query))
		if err != nil {
			yield(nil, err)
			return
		}

		summaries := make([]tracestore.TraceSummary, 0, len(dbSummaries))
		for _, dbSummary := range dbSummaries {
			summary, err := convertTraceSummaryFromDB(dbSummary)
			if err != nil {
				yield(nil, err)
				return
			}
			summaries = append(summaries, summary)
		}
		yield(summaries, nil)
	}
}

func convertTraceSummaryFromDB(dbSummary dbmodel.TraceSummary) (tracestore.TraceSummary, error) {
	traceID, err := convertTraceIDFromDB(dbSummary.TraceID)
	if err != nil {
		return tracestore.TraceSummary{}, err
	}

	services := make([]tracestore.ServiceSummary, 0, len(dbSummary.Services))
	for _, svc := range dbSummary.Services {
		services = append(services, tracestore.ServiceSummary{
			Name:           svc.ServiceName,
			SpanCount:      svc.SpanCount,
			ErrorSpanCount: svc.ErrorSpanCount,
		})
	}

	var minStart, maxEnd time.Time
	if dbSummary.MinStartTime > 0 {
		//nolint:gosec // G115: microsecond epoch timestamp is well within int64 range
		minStart = time.UnixMicro(int64(dbSummary.MinStartTime)).UTC()
	}
	if dbSummary.MaxEndTime > 0 {
		//nolint:gosec // G115: microsecond epoch timestamp is well within int64 range
		maxEnd = time.UnixMicro(int64(dbSummary.MaxEndTime)).UTC()
	}

	return tracestore.TraceSummary{
		TraceID:           traceID,
		RootServiceName:   dbSummary.RootServiceName,
		RootOperationName: dbSummary.RootOperationName,
		MinStartTime:      minStart,
		MaxEndTime:        maxEnd,
		SpanCount:         dbSummary.SpanCount,
		ErrorSpanCount:    dbSummary.ErrorSpanCount,
		// OrphanSpanCount is left at its zero value: the native aggregation cannot
		// compute it (see core.FindTraceSummaries).
		Services: services,
	}, nil
}
