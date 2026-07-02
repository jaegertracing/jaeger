// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/olivere/elastic/v7"
	"go.uber.org/zap"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

// maxServicesPerTrace caps the distinct services reported per trace. A trace with
// more services than this has its service list silently truncated by the terms
// aggregation; the value is high enough that real traces never hit it.
const maxServicesPerTrace = 1000

// FindTraceSummaries natively computes per-trace summaries (ADR-010 Milestone 5)
// via aggregations instead of fetching full span documents.
//
// It runs in two phases to keep the same semantics as the client-side fallback
// (computeSummaries over FindTraces): the query filter selects which traces match,
// but the summary of each matching trace must be computed over ALL of its spans,
// not just the spans that matched the filter. Phase 1 discovers the matching trace
// IDs; phase 2 aggregates over every span of those traces. Without this split, a
// filter on (say) a child service or a tag would summarize only the matching spans,
// making SpanCount, Services, root fields, error counts and max end time partial.
//
// OrphanSpanCount is left zero: identifying spans whose parent is absent requires
// a self-join over spans, which Elasticsearch aggregations cannot express; the
// client-side fallback computes it for backends that need it.
func (s *SpanReader) FindTraceSummaries(
	ctx context.Context,
	traceQuery dbmodel.TraceQueryParameters,
) ([]dbmodel.TraceSummary, error) {
	ctx, span := s.tracer.Start(ctx, "FindTraceSummaries")
	defer span.End()

	// Phase 1: discover the trace IDs matching the full query filter. FindTraceIDs
	// validates the query and applies the default search depth, exactly as the
	// FindTraces path does, so neither is duplicated here.
	traceIDs, err := s.FindTraceIDs(ctx, traceQuery)
	if err != nil {
		return nil, err
	}
	if len(traceIDs) == 0 {
		return []dbmodel.TraceSummary{}, nil
	}

	// Phase 2: aggregate over ALL spans of the matched traces. The time range is
	// padded the same way as multiRead so that spans falling slightly outside the
	// query window are still included, yielding the same full-trace view as the
	// FindTraces-based fallback. The aggregation is sized to the matched trace
	// count, since phase 2 only aggregates over those traces.
	const aggName = "trace_summaries"
	aggregation := s.buildTraceSummariesAggregation(len(traceIDs))
	boolQuery := s.buildTraceSummariesByIDsQuery(traceIDs, traceQuery.StartTimeMin, traceQuery.StartTimeMax)
	jaegerIndices := s.spanRotation.ReadTargets(
		traceQuery.StartTimeMin.Add(-s.maxTraceDuration),
		traceQuery.StartTimeMax.Add(s.maxTraceDuration),
	)

	searchResult, err := s.client().Search(jaegerIndices...).
		Size(0).
		Aggregation(aggName, aggregation).
		IgnoreUnavailable(true).
		Query(boolQuery).
		Do(ctx)
	if err != nil {
		err = es.DetailedError(err)
		s.logger.Info("es search for trace summaries failed", zap.Any("traceQuery", traceQuery), zap.Error(err))
		return nil, fmt.Errorf("search for trace summaries failed: %w", err)
	}

	buckets, found := searchResult.Aggregations.Terms(aggName)
	if !found {
		return nil, fmt.Errorf("could not find aggregation %q", aggName)
	}
	// Buckets arrive most-recent-first, ordered by the aggregation's max_start sort.
	return parseTraceSummaries(buckets)
}

// buildTraceSummariesByIDsQuery selects every span belonging to the given traces
// within a padded time window, so the summary aggregation runs over full traces.
func (s *SpanReader) buildTraceSummariesByIDsQuery(traceIDs []dbmodel.TraceID, startMin, startMax time.Time) elastic.Query {
	ids := make([]any, len(traceIDs))
	for i, id := range traceIDs {
		ids[i] = string(id)
	}
	// Mirror multiRead's ±maxTraceDuration padding so a trace's earlier/later spans
	// are included, and so this filter window matches the indices selected by
	// ReadTargets above (otherwise spans in adjacent indices would be filtered in but
	// never searched, yielding partial summaries).
	startTimeQuery := s.buildStartTimeQuery(startMin.Add(-s.maxTraceDuration), startMax.Add(s.maxTraceDuration))
	return elastic.NewBoolQuery().
		Must(elastic.NewTermsQuery(traceIDField, ids...)).
		Must(startTimeQuery)
}

func (s *SpanReader) buildTraceSummariesAggregation(numOfTraces int) elastic.Aggregation {
	// "error"="true" is the canonical boolean error tag the v2 ES writer emits for
	// spans with OTEL StatusCode=ERROR (see to_dbmodel.go).
	errorFilter := s.buildTagQuery("error", "true")

	services := elastic.NewTermsAggregation().
		Field(serviceNameField).
		Size(maxServicesPerTrace).
		SubAggregation("service_errors", elastic.NewFilterAggregation().Filter(errorFilter))

	// The root span is the one without a parent. Since #8859 the write path stores
	// parentSpanID only for non-root spans, so an existence filter selects the root
	// directly in Elasticsearch and the nested top_hits returns the earliest root's
	// service and operation. Spans written before #8859 carry no parentSpanID and
	// fall back to the earliest span of the trace.
	rootSpan := elastic.NewFilterAggregation().
		Filter(elastic.NewBoolQuery().MustNot(elastic.NewExistsQuery(parentSpanIDField))).
		SubAggregation("root_hit", elastic.NewTopHitsAggregation().
			Size(1).
			Sort(startTimeField, true). // earliest root first
			FetchSourceContext(elastic.NewFetchSourceContext(true).
				Include(serviceNameField, operationNameField)))

	return elastic.NewTermsAggregation().
		Field(traceIDField).
		Size(numOfTraces).
		Order("max_start", false). // most recent traces first
		SubAggregation("min_start", elastic.NewMinAggregation().Field(startTimeField)).
		SubAggregation("max_start", elastic.NewMaxAggregation().Field(startTimeField)).
		// max_end reads the denormalized endTime field, so no Painless script is needed.
		SubAggregation("max_end", elastic.NewMaxAggregation().Field(endTimeField)).
		SubAggregation("error_count", elastic.NewFilterAggregation().Filter(errorFilter)).
		SubAggregation("services", services).
		SubAggregation("root_span", rootSpan)
}

func parseTraceSummaries(buckets *elastic.AggregationBucketKeyItems) ([]dbmodel.TraceSummary, error) {
	summaries := make([]dbmodel.TraceSummary, 0, len(buckets.Buckets))
	for _, bucket := range buckets.Buckets {
		traceID, ok := bucket.Key.(string)
		if !ok {
			return nil, errors.New("non-string trace ID key in summary aggregation")
		}

		summary := dbmodel.TraceSummary{
			TraceID:   dbmodel.TraceID(traceID),
			SpanCount: int(bucket.DocCount),
		}
		if minStart, ok := bucket.Min("min_start"); ok && minStart.Value != nil {
			summary.MinStartTime = uint64(*minStart.Value)
		}
		if maxEnd, ok := bucket.Max("max_end"); ok && maxEnd.Value != nil {
			summary.MaxEndTime = uint64(*maxEnd.Value)
		}
		if errorCount, ok := bucket.Filter("error_count"); ok {
			summary.ErrorSpanCount = int(errorCount.DocCount)
		}
		services, err := parseServiceSummaries(bucket)
		if err != nil {
			return nil, fmt.Errorf("trace %s: %w", traceID, err)
		}
		summary.Services = services
		rootService, rootOperation, err := parseRootSpan(bucket)
		if err != nil {
			return nil, fmt.Errorf("trace %s: %w", traceID, err)
		}
		summary.RootServiceName, summary.RootOperationName = rootService, rootOperation

		summaries = append(summaries, summary)
	}
	return summaries, nil
}

func parseServiceSummaries(bucket *elastic.AggregationBucketKeyItem) ([]dbmodel.ServiceSummary, error) {
	servicesAgg, ok := bucket.Terms("services")
	if !ok {
		return nil, nil
	}
	services := make([]dbmodel.ServiceSummary, 0, len(servicesAgg.Buckets))
	for _, serviceBucket := range servicesAgg.Buckets {
		name, ok := serviceBucket.Key.(string)
		if !ok {
			return nil, errors.New("non-string service name in summary aggregation")
		}
		svc := dbmodel.ServiceSummary{
			ServiceName: name,
			SpanCount:   int(serviceBucket.DocCount),
		}
		if errs, ok := serviceBucket.Filter("service_errors"); ok {
			svc.ErrorSpanCount = int(errs.DocCount)
		}
		services = append(services, svc)
	}
	slices.SortFunc(services, func(a, b dbmodel.ServiceSummary) int {
		return cmp.Compare(a.ServiceName, b.ServiceName)
	})
	return services, nil
}

// rootSpanSource is the projection of the root span's _source.
type rootSpanSource struct {
	OperationName string `json:"operationName"`
	Process       struct {
		ServiceName string `json:"serviceName"`
	} `json:"process"`
}

// parseRootSpan returns the service and operation of the trace's root span, taken
// from the earliest span that has no parentSpanID (see buildTraceSummariesAggregation).
//
// Empty values with a nil error are returned when the trace has no parentless span
// (a valid outcome); a malformed top-hit _source is surfaced as an error rather
// than dropped.
func parseRootSpan(bucket *elastic.AggregationBucketKeyItem) (serviceName, operationName string, err error) {
	rootSpan, ok := bucket.Filter("root_span")
	if !ok {
		return "", "", nil
	}
	topHits, ok := rootSpan.TopHits("root_hit")
	if !ok || topHits.Hits == nil || len(topHits.Hits.Hits) == 0 {
		return "", "", nil
	}
	if topHits.Hits.Hits[0].Source == nil {
		return "", "", errors.New("root span top-hit missing _source")
	}
	var source rootSpanSource
	if err := json.Unmarshal(topHits.Hits.Hits[0].Source, &source); err != nil {
		return "", "", fmt.Errorf("failed to decode root span source: %w", err)
	}
	return source.Process.ServiceName, source.OperationName, nil
}
