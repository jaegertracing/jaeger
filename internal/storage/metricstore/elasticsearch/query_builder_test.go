// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	esmetrics "github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
)

var commonTimeRange = TimeRange{
	extendedStartTimeMillis: 1000,
	endTimeMillis:           2000,
}

// Test helper functions
func setupTestQB() *QueryBuilder {
	return NewQueryBuilder(nil, config.Configuration{Tags: config.TagsAsFields{DotReplacement: "_"}}, zap.NewNop())
}

func testAggregationStructure(t *testing.T, agg elastic.Aggregation, expectedInterval string, validateSubAggs func(map[string]any)) {
	src, err := agg.Source()
	require.NoError(t, err)

	aggMap, ok := src.(map[string]any)
	require.True(t, ok)

	dateHist, ok := aggMap["date_histogram"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, expectedInterval, dateHist["fixed_interval"])

	if validateSubAggs != nil {
		validateSubAggs(aggMap)
	}
}

// Tests
func TestBuildBoolQuery(t *testing.T) {
	qb := setupTestQB()
	params := metricstore.BaseQueryParameters{
		ServiceNames: []string{"service1", "service2"},
		SpanKinds:    []string{"client", "server"},
	}

	boolQuery := qb.BuildBoolQuery(params, commonTimeRange)
	require.NotNil(t, boolQuery)

	src, err := boolQuery.Source()
	require.NoError(t, err)

	queryMap := src.(map[string]any)
	boolClause := queryMap["bool"].(map[string]any)
	filterClause := boolClause["filter"].([]any)

	require.Len(t, filterClause, 3) // services, span kinds, time range
}

func TestBuildBoolQueryWithTags(t *testing.T) {
	qb := setupTestQB()
	params := metricstore.BaseQueryParameters{
		ServiceNames: []string{"service1"},
		SpanKinds:    []string{"server"},
		Tags: map[string]string{
			"environment": "prod",
			"region":      "us-east-1",
		},
	}

	boolQuery := qb.BuildBoolQuery(params, commonTimeRange)
	require.NotNil(t, boolQuery)

	src, err := boolQuery.Source()
	require.NoError(t, err)

	queryMap := src.(map[string]any)
	boolClause := queryMap["bool"].(map[string]any)
	filterClause := boolClause["filter"].([]any)

	require.Len(t, filterClause, 5) // services, span kinds, 2 tags, time range

	// Validate the presence of tag filters
	tagFiltersFound := 0
	for _, filter := range filterClause {
		filterMap, ok := filter.(map[string]any)
		if !ok {
			continue
		}

		if termQuery, ok := filterMap["term"]; ok {
			termMap, ok := termQuery.(map[string]any)
			if !ok {
				continue
			}

			if _, ok := termMap["tag.environment"]; ok {
				tagFiltersFound++
			}
			if _, ok := termMap["tag.region"]; ok {
				tagFiltersFound++
			}
		}
	}

	require.Equal(t, 2, tagFiltersFound, "Expected to find 2 tag filters")
}

func TestBuildLatenciesAggregation(t *testing.T) {
	qb := setupTestQB()
	step := time.Minute
	params := &metricstore.LatenciesQueryParameters{
		BaseQueryParameters: metricstore.BaseQueryParameters{
			Step: &step,
		},
		Quantile: 0.95,
	}

	agg := qb.BuildLatenciesAggQuery(params, commonTimeRange)
	require.NotNil(t, agg)

	testAggregationStructure(t, agg, "60000ms", func(aggMap map[string]any) {
		_, ok := aggMap["aggregations"].(map[string]any)
		require.True(t, ok)
	})
}

func TestBuildCallRateAggregation(t *testing.T) {
	qb := setupTestQB()
	step := time.Minute
	params := metricstore.BaseQueryParameters{
		Step: &step,
	}

	agg := qb.BuildCallRateAggQuery(params, commonTimeRange)
	require.NotNil(t, agg)

	testAggregationStructure(t, agg, "60000ms", func(aggMap map[string]any) {
		require.NotNil(t, aggMap["aggregations"])
	})
}

func TestBuildTimeSeriesAggQuery(t *testing.T) {
	qb := setupTestQB()
	step := time.Minute
	params := metricstore.BaseQueryParameters{
		Step:             &step,
		GroupByOperation: false,
	}
	subAgg := elastic.NewCumulativeSumAggregation()

	agg := qb.buildTimeSeriesAggQuery(params, commonTimeRange, "test_sub_agg", subAgg)
	require.NotNil(t, agg)

	testAggregationStructure(t, agg, "60000ms", func(aggMap map[string]any) {
		aggs := aggMap["aggregations"].(map[string]any)
		require.NotNil(t, aggs["test_sub_agg"])
	})
}

func TestExecute(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		sendResponse(t, w, mockEsValidResponse)
	}))
	defer mockServer.Close()

	cfg := &config.Configuration{
		Indices:  config.Indices{IndexPrefix: "test-jaeger"},
		Servers:  []string{mockServer.URL},
		LogLevel: "debug",
	}
	client := clientProvider(t, cfg, zap.NewNop(), esmetrics.NullFactory)
	qb := NewQueryBuilder(client, *cfg, zap.NewNop())

	boolQuery := elastic.NewBoolQuery()
	aggQuery := elastic.NewDateHistogramAggregation().Field("startTimeMillis").FixedInterval("60000ms")

	result, err := qb.Execute(context.Background(), *boolQuery, aggQuery, TimeRange{endTimeMillis: 0, startTimeMillis: 0})

	require.NoError(t, err)
	require.NotNil(t, result)
}
