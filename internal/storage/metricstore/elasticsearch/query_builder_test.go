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
	"github.com/stretchr/testify/assert"
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

// testIndicesConfig returns a config with deliberately different Spans and Services
// index settings so that tests will catch any mix-up between the two.
var testIndicesConfig = config.Indices{
	IndexPrefix: "test-jaeger",
	Spans: config.IndexOptions{
		DateLayout:        "2006-01-02",
		RolloverFrequency: "day",
	},
	Services: config.IndexOptions{
		DateLayout:        "2006-01-02-15",
		RolloverFrequency: "hour",
	},
}

// Test helper functions
func setupTestQB() *QueryBuilder {
	return NewQueryBuilder(nil, config.Configuration{
		Indices: testIndicesConfig,
		Tags:    config.TagsAsFields{DotReplacement: "_"},
	}, zap.NewNop())
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
	var requestPath string
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		sendResponse(t, w, mockEsValidResponse)
	}))
	defer mockServer.Close()

	cfg := &config.Configuration{
		Indices:  testIndicesConfig,
		Servers:  []string{mockServer.URL},
		LogLevel: "debug",
	}
	client := clientProvider(t, cfg, zap.NewNop(), esmetrics.NullFactory)
	qb := NewQueryBuilder(client, *cfg, zap.NewNop())

	boolQuery := elastic.NewBoolQuery()
	aggQuery := elastic.NewDateHistogramAggregation().Field("startTimeMillis").FixedInterval("60000ms")

	// Use epoch zero — with daily layout (Spans) this produces "1970-01-01",
	// with hourly layout (Services) it would produce "1970-01-01-00".
	result, err := qb.Execute(context.Background(), *boolQuery, aggQuery, TimeRange{endTimeMillis: 0, startTimeMillis: 0})

	require.NoError(t, err)
	require.NotNil(t, result)
	// Assert the span index date layout (daily) was used, not the service layout (hourly).
	assert.Contains(t, requestPath, "1970-01-01", "expected daily span index in path")
	assert.NotContains(t, requestPath, "1970-01-01-00", "got hourly service index in path, should use span index")
}
