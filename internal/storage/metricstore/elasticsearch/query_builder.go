// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/indices"
	esquery "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/query"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
)

// These constants define the specific names of aggregations used within Elasticsearch
// queries. They are crucial for both constructing the query sent to Elasticsearch
// and for correctly extracting the corresponding data from the Elasticsearch response.
const (
	aggName            = "results_buckets"
	culmuAggName       = "cumulative_requests"
	percentilesAggName = "percentiles_of_bucket"
	dateHistAggName    = "date_histogram"
)

// QueryBuilder is responsible for constructing Elasticsearch queries (bool and aggregation)
// based on provided parameters and executing them to retrieve raw search results.
type QueryBuilder struct {
	searcher     esclient.Searcher
	cfg          config.Configuration
	spanRotation indices.Rotation
}

// NewQueryBuilder creates a new QueryBuilder instance.
func NewQueryBuilder(searcher esclient.Searcher, cfg config.Configuration, spanRotation indices.Rotation) *QueryBuilder {
	return &QueryBuilder{
		searcher:     searcher,
		cfg:          cfg,
		spanRotation: spanRotation,
	}
}

func (q *QueryBuilder) BuildErrorBoolQuery(params metricstore.BaseQueryParameters, timeRange TimeRange) *esquery.BoolQuery {
	errorQuery := esquery.NewTermQuery("tag.error", true)
	return q.BuildBoolQuery(params, timeRange, errorQuery)
}

// BuildBoolQuery constructs the base bool query for filtering metrics data.
func (q *QueryBuilder) BuildBoolQuery(params metricstore.BaseQueryParameters, timeRange TimeRange, termsQueries ...esquery.Query) *esquery.BoolQuery {
	boolQuery := esquery.NewBoolQuery()

	serviceNameQuery := esquery.NewTermsQuery("process.serviceName", buildInterfaceSlice(params.ServiceNames)...)
	boolQuery.Filter(serviceNameQuery)

	spanKindField := strings.ReplaceAll(model.SpanKindKey, ".", q.cfg.Tags.DotReplacement)
	spanKindQuery := esquery.NewTermsQuery("tag."+spanKindField, buildInterfaceSlice(normalizeSpanKinds(params.SpanKinds))...)
	boolQuery.Filter(spanKindQuery)

	// Add additional terms queries if provided
	for _, termQuery := range termsQueries {
		boolQuery.Filter(termQuery)
	}

	rangeQuery := esquery.NewRangeQuery("startTimeMillis").
		Gte(timeRange.extendedStartTimeMillis).
		Lte(timeRange.endTimeMillis).
		Format("epoch_millis")
	boolQuery.Filter(rangeQuery)

	return boolQuery
}

// BuildLatenciesAggQuery constructs the aggregation query for latency metrics.
func (q *QueryBuilder) BuildLatenciesAggQuery(params *metricstore.LatenciesQueryParameters, timeRange TimeRange) esquery.Aggregation {
	percentilesAgg := esquery.NewPercentilesAggregation().
		Field("duration").
		Percentiles(params.Quantile * 100)
	return q.buildTimeSeriesAggQuery(params.BaseQueryParameters, timeRange, percentilesAggName, percentilesAgg)
}

// BuildCallRateAggQuery constructs the aggregation query for call rate metrics.
func (q *QueryBuilder) BuildCallRateAggQuery(params metricstore.BaseQueryParameters, timeRange TimeRange) esquery.Aggregation {
	cumulativeSumAgg := esquery.NewCumulativeSumAggregation().BucketsPath("_count")
	return q.buildTimeSeriesAggQuery(params, timeRange, culmuAggName, cumulativeSumAgg)
}

// buildTimeSeriesAggQuery constructs a time series aggregation with a sub-aggregation.
func (*QueryBuilder) buildTimeSeriesAggQuery(params metricstore.BaseQueryParameters, timeRange TimeRange, subAggName string, subAgg esquery.Aggregation) esquery.Aggregation {
	fixedIntervalString := strconv.FormatInt(params.Step.Milliseconds(), 10) + "ms"

	dateHistAgg := esquery.NewDateHistogramAggregation().
		Field("startTimeMillis").
		FixedInterval(fixedIntervalString).
		MinDocCount(0).
		ExtendedBounds(timeRange.extendedStartTimeMillis, timeRange.endTimeMillis).
		SubAggregation(subAggName, subAgg)

	if params.GroupByOperation {
		return esquery.NewTermsAggregation("operationName").
			Size(10).
			SubAggregation(dateHistAggName, dateHistAgg)
	}
	return dateHistAgg
}

// Execute runs the Elasticsearch search with the provided bool and aggregation queries.
func (q *QueryBuilder) Execute(ctx context.Context, boolQuery *esquery.BoolQuery, aggQuery esquery.Aggregation, timeRange TimeRange) (*esclient.SearchResponse, error) {
	idxList := q.spanRotation.ReadTargets(
		time.UnixMilli(timeRange.extendedStartTimeMillis).UTC(),
		time.UnixMilli(timeRange.endTimeMillis).UTC(),
	)

	// Size 0 returns only aggregation results, excluding individual search hits.
	return q.searcher.Search(ctx, idxList, esclient.SearchRequest{
		Size:         0,
		Query:        boolQuery,
		Aggregations: map[string]esquery.Aggregation{aggName: aggQuery},
	})
}

// normalizeSpanKinds normalizes a slice of span kinds.
func normalizeSpanKinds(spanKinds []string) []string {
	normalized := make([]string, len(spanKinds))
	for i, kind := range spanKinds {
		normalized[i] = strings.ToLower(strings.TrimPrefix(kind, "SPAN_KIND_"))
	}
	return normalized
}

// buildInterfaceSlice converts []string to []any for a terms query.
func buildInterfaceSlice(s []string) []any {
	ifaceSlice := make([]any, len(s))
	for i, v := range s {
		ifaceSlice[i] = v
	}
	return ifaceSlice
}
