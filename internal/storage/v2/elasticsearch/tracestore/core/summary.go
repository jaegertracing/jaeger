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
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	esquery "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/query"
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

	searchResult, err := s.searcher.Search(ctx, jaegerIndices, esclient.SearchRequest{
		Size:  0,
		Query: boolQuery,
		Aggregations: map[string]esquery.Aggregation{
			aggName: aggregation,
		},
	})
	if err != nil {
		if isScriptingDisabledError(err) {
			// The max_end aggregation needs Painless scripting. When it is disabled,
			// returning ErrUnsupported lets the query service fall back to client-side
			// summary computation instead of failing the request.
			return nil, fmt.Errorf("native trace summaries require Painless scripting enabled on the cluster: %w", errors.ErrUnsupported)
		}
		s.logger.Info("es search for trace summaries failed", zap.Any("traceQuery", traceQuery), zap.Error(err))
		return nil, fmt.Errorf("search for trace summaries failed: %w", err)
	}

	buckets, found := searchResult.Aggregations.Terms(aggName)
	if !found {
		return nil, fmt.Errorf("could not find aggregation %q", aggName)
	}
	// Buckets arrive most-recent-first, ordered by the aggregation's max_start sort.
	return parseTraceSummaries(buckets.Buckets)
}

// isScriptingDisabledError reports whether an Elasticsearch/OpenSearch error was
// caused by inline (Painless) scripting being disabled on the cluster, which the
// max_end aggregation depends on. The cluster surfaces this as an
// illegal_argument_exception whose reason mentions scripts being disabled; the
// reason is carried in the raw response body of the client's ResponseError.
func isScriptingDisabledError(err error) bool {
	var respErr esclient.ResponseError
	if !errors.As(err, &respErr) || len(respErr.Body) == 0 {
		return false
	}
	var body struct {
		Error struct {
			Reason    string `json:"reason"`
			RootCause []struct {
				Reason string `json:"reason"`
			} `json:"root_cause"`
		} `json:"error"`
	}
	if json.Unmarshal(respErr.Body, &body) != nil {
		return false
	}
	reasons := make([]string, 0, len(body.Error.RootCause)+1)
	reasons = append(reasons, body.Error.Reason)
	for _, c := range body.Error.RootCause {
		reasons = append(reasons, c.Reason)
	}
	for _, reason := range reasons {
		reason = strings.ToLower(reason)
		if strings.Contains(reason, "script") &&
			(strings.Contains(reason, "disabled") || strings.Contains(reason, "cannot execute")) {
			return true
		}
	}
	return false
}

// buildTraceSummariesByIDsQuery selects every span belonging to the given traces
// within a padded time window, so the summary aggregation runs over full traces.
func (s *SpanReader) buildTraceSummariesByIDsQuery(traceIDs []dbmodel.TraceID, startMin, startMax time.Time) esquery.Query {
	ids := make([]any, len(traceIDs))
	for i, id := range traceIDs {
		ids[i] = string(id)
	}
	// Mirror multiRead's ±maxTraceDuration padding so a trace's earlier/later spans
	// are included, and so this filter window matches the indices selected by
	// ReadTargets above (otherwise spans in adjacent indices would be filtered in but
	// never searched, yielding partial summaries).
	startTimeQuery := s.buildStartTimeQuery(startMin.Add(-s.maxTraceDuration), startMax.Add(s.maxTraceDuration))
	return esquery.NewBoolQuery().
		Must(esquery.NewTermsQuery(traceIDField, ids...)).
		Must(startTimeQuery)
}

func (s *SpanReader) buildTraceSummariesAggregation(numOfTraces int) esquery.Aggregation {
	// "error"="true" is the canonical boolean error tag the v2 ES writer emits for
	// spans with OTEL StatusCode=ERROR (see to_dbmodel.go).
	errorFilter := s.buildTagQuery("error", "true")

	services := esquery.NewTermsAggregation(serviceNameField).
		Size(maxServicesPerTrace).
		SubAggregation("service_errors", esquery.NewFilterAggregation(errorFilter))

	// The root span is the one without a parent. Since #8859 the write path stores
	// parentSpanID only for non-root spans, so an existence filter selects the root
	// directly in Elasticsearch and the nested top_hits returns the earliest root's
	// service and operation. Spans written before #8859 carry no parentSpanID and
	// fall back to the earliest span of the trace.
	rootSpan := esquery.NewFilterAggregation(
		esquery.NewBoolQuery().MustNot(esquery.NewExistsQuery(parentSpanIDField)),
	).SubAggregation("root_hit", esquery.NewTopHitsAggregation().
		Size(1).
		Sort(startTimeField, esquery.Ascending). // earliest root first
		SourceIncludes(serviceNameField, operationNameField))

	return esquery.NewTermsAggregation(traceIDField).
		Size(numOfTraces).
		Order("max_start", esquery.Descending). // most recent traces first
		SubAggregation("min_start", esquery.NewMinAggregation(startTimeField)).
		SubAggregation("max_start", esquery.NewMaxAggregation(startTimeField)).
		// max_end is derived by script: ES persists no end-time field (end = start + duration).
		SubAggregation("max_end", esquery.NewScriptedMaxAggregation(
			"doc['"+startTimeField+"'].value + doc['"+durationField+"'].value",
		)).
		SubAggregation("error_count", esquery.NewFilterAggregation(errorFilter)).
		SubAggregation("services", services).
		SubAggregation("root_span", rootSpan)
}

func parseTraceSummaries(buckets []esclient.AggregationBucket) ([]dbmodel.TraceSummary, error) {
	summaries := make([]dbmodel.TraceSummary, 0, len(buckets))
	for _, bucket := range buckets {
		summary := dbmodel.TraceSummary{
			TraceID:   dbmodel.TraceID(bucket.Key),
			SpanCount: bucket.DocCount,
		}
		if minStart, ok := bucket.Metric("min_start"); ok && minStart != nil {
			summary.MinStartTime = uint64(*minStart)
		}
		if maxEnd, ok := bucket.Metric("max_end"); ok && maxEnd != nil {
			summary.MaxEndTime = uint64(*maxEnd)
		}
		if errorCount, ok := bucket.Filter("error_count"); ok {
			summary.ErrorSpanCount = errorCount.DocCount
		}
		summary.Services = parseServiceSummaries(bucket)
		rootService, rootOperation, err := parseRootSpan(bucket)
		if err != nil {
			return nil, fmt.Errorf("trace %s: %w", bucket.Key, err)
		}
		summary.RootServiceName, summary.RootOperationName = rootService, rootOperation
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

func parseServiceSummaries(bucket esclient.AggregationBucket) []dbmodel.ServiceSummary {
	servicesAgg, ok := bucket.Terms("services")
	if !ok {
		return nil
	}
	services := make([]dbmodel.ServiceSummary, 0, len(servicesAgg.Buckets))
	for _, serviceBucket := range servicesAgg.Buckets {
		svc := dbmodel.ServiceSummary{
			ServiceName: serviceBucket.Key,
			SpanCount:   serviceBucket.DocCount,
		}
		if errs, ok := serviceBucket.Filter("service_errors"); ok {
			svc.ErrorSpanCount = errs.DocCount
		}
		services = append(services, svc)
	}
	slices.SortFunc(services, func(a, b dbmodel.ServiceSummary) int {
		return cmp.Compare(a.ServiceName, b.ServiceName)
	})
	return services
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
func parseRootSpan(bucket esclient.AggregationBucket) (serviceName, operationName string, err error) {
	rootSpan, ok := bucket.Filter("root_span")
	if !ok {
		return "", "", nil
	}
	topHits, ok := rootSpan.TopHits("root_hit")
	if !ok || len(topHits.Hits) == 0 {
		return "", "", nil
	}
	source := topHits.Hits[0].Source
	if len(source) == 0 {
		return "", "", errors.New("root span top-hit missing _source")
	}
	var parsed rootSpanSource
	if err := json.Unmarshal(source, &parsed); err != nil {
		return "", "", fmt.Errorf("failed to decode root span source: %w", err)
	}
	return parsed.Process.ServiceName, parsed.OperationName, nil
}
