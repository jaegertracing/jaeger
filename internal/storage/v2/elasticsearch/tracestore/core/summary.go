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

	"github.com/olivere/elastic/v7"
	"go.uber.org/zap"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	cfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

const (
	traceSummariesAggregation   = "trace_summaries"
	minStartSubAggregation      = "min_start"
	maxStartSubAggregation      = "max_start"
	maxEndSubAggregation        = "max_end"
	errorCountSubAggregation    = "error_count"
	servicesSubAggregation      = "services"
	serviceErrorsSubAggregation = "service_errors"
	rootSubAggregation          = "root"
	rootHitSubAggregation       = "root_hit"

	nestedReferencesField = "references"
	referenceTypeField    = "refType"

	// The canonical "error" boolean tag the v2 ES writer emits for spans with
	// OTEL StatusCode = ERROR (see to_dbmodel.go).
	errorTagKey   = "error"
	errorTagValue = "true"

	maxServicesPerTrace = 1000

	// ElasticSearch persists no end-time field, so max end time (start + duration)
	// can only be derived via a script.
	endTimeScript = "doc['" + startTimeField + "'].value + doc['" + durationField + "'].value"
)

// FindTraceSummaries natively computes per-trace summaries (ADR-010 Milestone 5)
// via a single size=0 aggregation instead of fetching span documents.
//
// OrphanSpanCount is left zero: identifying spans whose parent is absent requires
// a self-join over spans, which ElasticSearch aggregations cannot express; the
// client-side fallback computes it for backends that need it.
func (s *SpanReader) FindTraceSummaries(
	ctx context.Context,
	traceQuery dbmodel.TraceQueryParameters,
) ([]dbmodel.TraceSummary, error) {
	ctx, span := s.tracer.Start(ctx, "FindTraceSummaries")
	defer span.End()

	if err := validateQuery(traceQuery); err != nil {
		return nil, err
	}
	if traceQuery.SearchDepth == 0 {
		traceQuery.SearchDepth = defaultSearchDepth
	}

	aggregation := s.buildTraceSummariesAggregation(traceQuery.SearchDepth)
	boolQuery := s.buildFindTraceIDsQuery(traceQuery)
	jaegerIndices := s.timeRangeIndices(
		s.spanIndexPrefix,
		s.spanIndex.DateLayout,
		traceQuery.StartTimeMin,
		traceQuery.StartTimeMax,
		cfg.RolloverFrequencyAsNegativeDuration(s.spanIndex.RolloverFrequency),
	)

	searchResult, err := s.client().Search(jaegerIndices...).
		Size(0).
		Aggregation(traceSummariesAggregation, aggregation).
		IgnoreUnavailable(true).
		Query(boolQuery).
		Do(ctx)
	if err != nil {
		err = es.DetailedError(err)
		s.logger.Info("es search for trace summaries failed", zap.Any("traceQuery", traceQuery), zap.Error(err))
		return nil, fmt.Errorf("search for trace summaries failed: %w", err)
	}

	if searchResult.Aggregations == nil {
		return []dbmodel.TraceSummary{}, nil
	}
	buckets, found := searchResult.Aggregations.Terms(traceSummariesAggregation)
	if !found {
		return nil, ErrUnableToFindTraceIDAggregation
	}
	return parseTraceSummaries(buckets)
}

func (s *SpanReader) buildTraceSummariesAggregation(numOfTraces int) elastic.Aggregation {
	errorFilter := s.buildTagQuery(errorTagKey, errorTagValue)

	services := elastic.NewTermsAggregation().
		Field(serviceNameField).
		Size(maxServicesPerTrace).
		SubAggregation(serviceErrorsSubAggregation, elastic.NewFilterAggregation().Filter(errorFilter))

	rootHit := elastic.NewTopHitsAggregation().
		Size(1).
		Sort(startTimeField, true).
		FetchSourceContext(elastic.NewFetchSourceContext(true).Include(serviceNameField, operationNameField))
	root := elastic.NewFilterAggregation().
		Filter(buildRootSpanQuery()).
		SubAggregation(rootHitSubAggregation, rootHit)

	return elastic.NewTermsAggregation().
		Field(traceIDField).
		Size(numOfTraces).
		Order(maxStartSubAggregation, false). // most recent traces first
		SubAggregation(minStartSubAggregation, elastic.NewMinAggregation().Field(startTimeField)).
		SubAggregation(maxStartSubAggregation, elastic.NewMaxAggregation().Field(startTimeField)).
		SubAggregation(maxEndSubAggregation, elastic.NewMaxAggregation().Script(elastic.NewScript(endTimeScript))).
		SubAggregation(errorCountSubAggregation, elastic.NewFilterAggregation().Filter(errorFilter)).
		SubAggregation(servicesSubAggregation, services).
		SubAggregation(rootSubAggregation, root)
}

// buildRootSpanQuery matches root spans: the v2 ES writer encodes a span's parent
// as a CHILD_OF entry in the nested references array, so its absence marks a root.
func buildRootSpanQuery() elastic.Query {
	childOf := elastic.NewNestedQuery(
		nestedReferencesField,
		elastic.NewTermQuery(nestedReferencesField+"."+referenceTypeField, string(dbmodel.ChildOf)),
	)
	return elastic.NewBoolQuery().MustNot(childOf)
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
		if minStart, ok := bucket.Min(minStartSubAggregation); ok && minStart.Value != nil {
			summary.MinStartTime = uint64(*minStart.Value)
		}
		if maxEnd, ok := bucket.Max(maxEndSubAggregation); ok && maxEnd.Value != nil {
			summary.MaxEndTime = uint64(*maxEnd.Value)
		}
		if errorCount, ok := bucket.Filter(errorCountSubAggregation); ok {
			summary.ErrorSpanCount = int(errorCount.DocCount)
		}
		summary.Services = parseServiceSummaries(bucket)
		rootService, rootOperation, err := parseRootSpan(bucket)
		if err != nil {
			return nil, fmt.Errorf("trace %s: %w", traceID, err)
		}
		summary.RootServiceName, summary.RootOperationName = rootService, rootOperation

		summaries = append(summaries, summary)
	}
	return summaries, nil
}

func parseServiceSummaries(bucket *elastic.AggregationBucketKeyItem) []dbmodel.ServiceSummary {
	servicesAgg, ok := bucket.Terms(servicesSubAggregation)
	if !ok {
		return nil
	}
	services := make([]dbmodel.ServiceSummary, 0, len(servicesAgg.Buckets))
	for _, serviceBucket := range servicesAgg.Buckets {
		name, _ := serviceBucket.Key.(string)
		svc := dbmodel.ServiceSummary{
			ServiceName: name,
			SpanCount:   int(serviceBucket.DocCount),
		}
		if errs, ok := serviceBucket.Filter(serviceErrorsSubAggregation); ok {
			svc.ErrorSpanCount = int(errs.DocCount)
		}
		services = append(services, svc)
	}
	slices.SortFunc(services, func(a, b dbmodel.ServiceSummary) int {
		return cmp.Compare(a.ServiceName, b.ServiceName)
	})
	return services
}

// parseRootSpan returns empty values with a nil error when the bucket has no root
// span (a valid outcome); a malformed top-hit _source is surfaced as an error
// rather than silently dropped.
func parseRootSpan(bucket *elastic.AggregationBucketKeyItem) (serviceName, operationName string, err error) {
	rootAgg, ok := bucket.Filter(rootSubAggregation)
	if !ok {
		return "", "", nil
	}
	topHits, ok := rootAgg.TopHits(rootHitSubAggregation)
	if !ok || topHits.Hits == nil || len(topHits.Hits.Hits) == 0 {
		return "", "", nil
	}
	var source struct {
		OperationName string `json:"operationName"`
		Process       struct {
			ServiceName string `json:"serviceName"`
		} `json:"process"`
	}
	if err := json.Unmarshal(topHits.Hits.Hits[0].Source, &source); err != nil {
		return "", "", fmt.Errorf("failed to decode root span source: %w", err)
	}
	return source.Process.ServiceName, source.OperationName, nil
}
