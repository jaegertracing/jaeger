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

const (
	traceSummariesAggregation    = "trace_summaries"
	minStartSubAggregation       = "min_start"
	maxStartSubAggregation       = "max_start"
	maxEndSubAggregation         = "max_end"
	errorCountSubAggregation     = "error_count"
	servicesSubAggregation       = "services"
	serviceErrorsSubAggregation  = "service_errors"
	rootCandidatesSubAggregation = "root_candidates"

	nestedReferencesField = "references"

	// The canonical "error" boolean tag the v2 ES writer emits for spans with
	// OTEL StatusCode = ERROR (see to_dbmodel.go).
	errorTagKey   = "error"
	errorTagValue = "true"

	maxServicesPerTrace = 1000

	// maxRootCandidates bounds how many of the earliest spans per trace are fetched
	// to locate the root. The root is the earliest span with no in-trace parent, so
	// inspecting a small window of the earliest spans is sufficient in practice; the
	// size=0 aggregation never fetches full traces. See FindTraceSummaries.
	maxRootCandidates = 10

	// ElasticSearch persists no end-time field, so max end time (start + duration)
	// can only be derived via a script.
	endTimeScript = "doc['" + startTimeField + "'].value + doc['" + durationField + "'].value"
)

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

	// Phase 1: discover the trace IDs matching the full query filter.
	traceIDs, err := s.findTraceIDsFromQuery(ctx, traceQuery)
	if err != nil {
		return nil, err
	}
	if len(traceIDs) == 0 {
		return []dbmodel.TraceSummary{}, nil
	}

	// Phase 2: aggregate over ALL spans of the matched traces. The time range is
	// padded the same way as multiRead so that spans falling slightly outside the
	// query window are still included, yielding the same full-trace view as the
	// FindTraces-based fallback.
	aggregation := s.buildTraceSummariesAggregation(traceQuery.SearchDepth)
	boolQuery := s.buildTraceSummariesByIDsQuery(traceIDs, traceQuery.StartTimeMin, traceQuery.StartTimeMax)
	jaegerIndices := s.spanRotation.ReadTargets(
		traceQuery.StartTimeMin.Add(-time.Hour),
		traceQuery.StartTimeMax.Add(time.Hour),
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

// buildTraceSummariesByIDsQuery selects every span belonging to the given traces
// within a padded time window, so the summary aggregation runs over full traces.
func (s *SpanReader) buildTraceSummariesByIDsQuery(traceIDs []dbmodel.TraceID, startMin, startMax time.Time) elastic.Query {
	ids := make([]any, len(traceIDs))
	for i, id := range traceIDs {
		ids[i] = string(id)
	}
	// Mirror multiRead's ±24h padding so a trace's earlier/later spans are included.
	startTimeQuery := s.buildStartTimeQuery(startMin.Add(-24*time.Hour), startMax.Add(24*time.Hour))
	return elastic.NewBoolQuery().
		Must(elastic.NewTermsQuery(traceIDField, ids...)).
		Must(startTimeQuery)
}

func (s *SpanReader) buildTraceSummariesAggregation(numOfTraces int) elastic.Aggregation {
	errorFilter := s.buildTagQuery(errorTagKey, errorTagValue)

	services := elastic.NewTermsAggregation().
		Field(serviceNameField).
		Size(maxServicesPerTrace).
		SubAggregation(serviceErrorsSubAggregation, elastic.NewFilterAggregation().Filter(errorFilter))

	// Fetch the earliest spans (with their references and trace ID) so the root can
	// be determined in Go using the same rule as getParentSpanId — a span is the
	// root when it has no reference to another span in its own trace. This is not
	// expressible as an ElasticSearch query because a nested reference's trace ID
	// cannot be compared against the parent document's trace ID. See parseRootSpan.
	rootCandidates := elastic.NewTopHitsAggregation().
		Size(maxRootCandidates).
		Sort(startTimeField, true). // earliest first
		FetchSourceContext(elastic.NewFetchSourceContext(true).
			Include(serviceNameField, operationNameField, traceIDField, nestedReferencesField))

	return elastic.NewTermsAggregation().
		Field(traceIDField).
		Size(numOfTraces).
		Order(maxStartSubAggregation, false). // most recent traces first
		SubAggregation(minStartSubAggregation, elastic.NewMinAggregation().Field(startTimeField)).
		SubAggregation(maxStartSubAggregation, elastic.NewMaxAggregation().Field(startTimeField)).
		SubAggregation(maxEndSubAggregation, elastic.NewMaxAggregation().Script(elastic.NewScript(endTimeScript))).
		SubAggregation(errorCountSubAggregation, elastic.NewFilterAggregation().Filter(errorFilter)).
		SubAggregation(servicesSubAggregation, services).
		SubAggregation(rootCandidatesSubAggregation, rootCandidates)
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

// rootSpanRef is the projection of a span reference needed for root detection.
// Only the referenced trace ID matters: a same-trace reference (of any type) means
// the span has a parent, while cross-trace references (span links) are ignored.
type rootSpanRef struct {
	TraceID string `json:"traceID"`
}

// rootCandidateSource is the projection of a candidate root span's _source.
type rootCandidateSource struct {
	OperationName string `json:"operationName"`
	TraceID       string `json:"traceID"`
	Process       struct {
		ServiceName string `json:"serviceName"`
	} `json:"process"`
	References []rootSpanRef `json:"references"`
}

// parseRootSpan returns the root span's service and operation, scanning the fetched
// candidates in ascending start-time order and selecting the first that is a root.
//
// Root-ness uses the same rule as getParentSpanId in the parent tracestore package:
// a span is a root when it has no reference to a span in its own trace. References
// to other traces (span links, including cross-trace CHILD_OF) are ignored, and a
// same-trace reference of any type (CHILD_OF or FOLLOWS_FROM) marks a non-root span.
// This keeps native summaries consistent with the client-side computeSummaries
// fallback, which picks the earliest span whose reconstructed parent is empty.
//
// Empty values with a nil error are returned when no root is found (a valid
// outcome); a malformed top-hit _source is surfaced as an error rather than dropped.
func parseRootSpan(bucket *elastic.AggregationBucketKeyItem) (serviceName, operationName string, err error) {
	topHits, ok := bucket.TopHits(rootCandidatesSubAggregation)
	if !ok || topHits.Hits == nil {
		return "", "", nil
	}
	for _, hit := range topHits.Hits.Hits {
		var source rootCandidateSource
		if err := json.Unmarshal(hit.Source, &source); err != nil {
			return "", "", fmt.Errorf("failed to decode root span source: %w", err)
		}
		if isRootSpan(source.TraceID, source.References) {
			return source.Process.ServiceName, source.OperationName, nil
		}
	}
	return "", "", nil
}

// isRootSpan reports whether a span with the given trace ID and references is a
// trace root, i.e. it has no reference pointing to a span in its own trace. This
// mirrors getParentSpanId (parent empty <=> no same-trace reference).
func isRootSpan(traceID string, references []rootSpanRef) bool {
	for _, ref := range references {
		if ref.TraceID == traceID {
			return false
		}
	}
	return true
}
