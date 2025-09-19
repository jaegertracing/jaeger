// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/olivere/elastic/v7"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	esquery "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/query"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore"
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
	client           es.Client
	cfg              config.Configuration
	timeRangeIndices spanstore.TimeRangeIndexFn
	spanReader       *spanstore.SpanReader
}

// NewQueryBuilder creates a new QueryBuilder instance.
func NewQueryBuilder(client es.Client, cfg config.Configuration, logger *zap.Logger) *QueryBuilder {
	// Create SpanReader parameters for reusing tag query logic
	spanReaderParams := spanstore.SpanReaderParams{
		Client:              func() es.Client { return client },
		Logger:              logger,
		IndexPrefix:         cfg.Indices.IndexPrefix,
		TagDotReplacement:   cfg.Tags.DotReplacement,
		UseReadWriteAliases: cfg.UseReadWriteAliases,
		ReadAliasSuffix:     cfg.ReadAliasSuffix,
		RemoteReadClusters:  cfg.RemoteReadClusters,
		MaxSpanAge:          24 * time.Hour, // Default value
		MaxDocCount:         10000,          // Default value
	}

	return &QueryBuilder{
		client: client,
		cfg:    cfg,
		timeRangeIndices: spanstore.LoggingTimeRangeIndexFn(
			logger,
			spanstore.TimeRangeIndicesFn(cfg.UseReadWriteAliases, cfg.ReadAliasSuffix, cfg.RemoteReadClusters),
		),
		spanReader: spanstore.NewSpanReader(spanReaderParams),
	}
}

func (q *QueryBuilder) BuildErrorBoolQuery(params metricstore.BaseQueryParameters, timeRange TimeRange) elastic.BoolQuery {
	errorQuery := elastic.NewTermQuery("tag.error", true)
	return q.BuildBoolQuery(params, timeRange, errorQuery)
}

// BuildBoolQuery constructs the base bool query for filtering metrics data.
func (q *QueryBuilder) BuildBoolQuery(params metricstore.BaseQueryParameters, timeRange TimeRange, termsQueries ...elastic.Query) elastic.BoolQuery {
	boolQuery := elastic.NewBoolQuery()

	serviceNameQuery := elastic.NewTermsQuery("process.serviceName", buildInterfaceSlice(params.ServiceNames)...)
	boolQuery.Filter(serviceNameQuery)

	spanKindField := strings.ReplaceAll(model.SpanKindKey, ".", q.cfg.Tags.DotReplacement)
	spanKindQuery := elastic.NewTermsQuery("tag."+spanKindField, buildInterfaceSlice(normalizeSpanKinds(params.SpanKinds))...)
	boolQuery.Filter(spanKindQuery)

	// Add complex tag filters using SpanReader's buildTagQuery method
	for tagKey, tagValue := range params.Tags {
		tagQuery := q.spanReader.BuildTagQuery(tagKey, tagValue)
		boolQuery.Filter(tagQuery)
	}

	// Add additional terms queries if provided
	for _, termQuery := range termsQueries {
		boolQuery.Filter(termQuery)
	}

	rangeQuery := esquery.NewRangeQuery("startTimeMillis").
		Gte(timeRange.extendedStartTimeMillis).
		Lte(timeRange.endTimeMillis).
		Format("epoch_millis")
	boolQuery.Filter(rangeQuery)

	return *boolQuery
}

// BuildLatenciesAggQuery constructs the aggregation query for latency metrics.
func (q *QueryBuilder) BuildLatenciesAggQuery(params *metricstore.LatenciesQueryParameters, timeRange TimeRange) elastic.Aggregation {
	percentilesAgg := elastic.NewPercentilesAggregation().
		Field("duration").
		Percentiles(params.Quantile * 100)
	return q.buildTimeSeriesAggQuery(params.BaseQueryParameters, timeRange, percentilesAggName, percentilesAgg)
}

// BuildCallRateAggQuery constructs the aggregation query for call rate metrics.
func (q *QueryBuilder) BuildCallRateAggQuery(params metricstore.BaseQueryParameters, timeRange TimeRange) elastic.Aggregation {
	cumulativeSumAgg := elastic.NewCumulativeSumAggregation().BucketsPath("_count")
	return q.buildTimeSeriesAggQuery(params, timeRange, culmuAggName, cumulativeSumAgg)
}

// buildTimeSeriesAggQuery constructs a time series aggregation with a sub-aggregation.
func (*QueryBuilder) buildTimeSeriesAggQuery(params metricstore.BaseQueryParameters, timeRange TimeRange, subAggName string, subAgg elastic.Aggregation) elastic.Aggregation {
	fixedIntervalString := strconv.FormatInt(params.Step.Milliseconds(), 10) + "ms"

	dateHistAgg := elastic.NewDateHistogramAggregation().
		Field("startTimeMillis").
		FixedInterval(fixedIntervalString).
		MinDocCount(0).
		ExtendedBounds(timeRange.extendedStartTimeMillis, timeRange.endTimeMillis).
		SubAggregation(subAggName, subAgg)

	if params.GroupByOperation {
		return elastic.NewTermsAggregation().
			Field("operationName").
			Size(10).
			SubAggregation(dateHistAggName, dateHistAgg)
	}
	return dateHistAgg
}

// Execute runs the Elasticsearch search with the provided bool and aggregation queries.
func (q *QueryBuilder) Execute(ctx context.Context, boolQuery elastic.BoolQuery, aggQuery elastic.Aggregation, timeRange TimeRange) (*elastic.SearchResult, error) {
	indexName := q.cfg.Indices.IndexPrefix.Apply("jaeger-span-")
	indices := q.timeRangeIndices(
		indexName,
		q.cfg.Indices.Services.DateLayout,
		time.UnixMilli(timeRange.extendedStartTimeMillis).UTC(),
		time.UnixMilli(timeRange.endTimeMillis).UTC(),
		config.RolloverFrequencyAsNegativeDuration(q.cfg.Indices.Services.RolloverFrequency),
	)

	return q.client.Search(indices...).
		IgnoreUnavailable(true).
		Query(&boolQuery).
		Size(0). // Set Size to 0 to return only aggregation results, excluding individual search hits
		Aggregation(aggName, aggQuery).
		Do(ctx)
}

// normalizeSpanKinds normalizes a slice of span kinds.
func normalizeSpanKinds(spanKinds []string) []string {
	normalized := make([]string, len(spanKinds))
	for i, kind := range spanKinds {
		normalized[i] = strings.ToLower(strings.TrimPrefix(kind, "SPAN_KIND_"))
	}
	return normalized
}

// buildInterfaceSlice converts []string to []interface{} for elastic terms query.
func buildInterfaceSlice(s []string) []any {
	ifaceSlice := make([]any, len(s))
	for i, v := range s {
		ifaceSlice[i] = v
	}
	return ifaceSlice
}
