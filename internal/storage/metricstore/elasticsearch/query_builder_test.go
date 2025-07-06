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
	"go.uber.org/zap"

	esmetrics "github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
)

var (
	commonTimeRange = TimeRange{
		extendedStartTimeMillis: 1000,
		endTimeMillis:           2000,
	}

	commonStep = time.Minute
)

// Test helper functions
func setupTestQB() *QueryBuilder {
	return NewQueryBuilder(nil, config.Configuration{Tags: config.TagsAsFields{DotReplacement: "_"}})
}

func testAggregationStructure(t *testing.T, agg elastic.Aggregation, expectedInterval string, validateSubAggs func(map[string]interface{})) {
	src, err := agg.Source()
	assert.NoError(t, err)

	aggMap, ok := src.(map[string]interface{})
	assert.True(t, ok)

	dateHist, ok := aggMap["date_histogram"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, expectedInterval, dateHist["fixed_interval"])

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
	assert.NotNil(t, boolQuery)

	src, err := boolQuery.Source()
	assert.NoError(t, err)

	queryMap := src.(map[string]interface{})
	boolClause := queryMap["bool"].(map[string]interface{})
	filterClause := boolClause["filter"].([]interface{})

	assert.Len(t, filterClause, 3) // services, span kinds, time range
}

func TestBuildLatenciesAggregation(t *testing.T) {
	qb := setupTestQB()
	params := &metricstore.LatenciesQueryParameters{
		BaseQueryParameters: metricstore.BaseQueryParameters{
			Step: &commonStep,
		},
		Quantile: 0.95,
	}

	agg := qb.BuildLatenciesAggQuery(params, commonTimeRange)
	assert.NotNil(t, agg)

	testAggregationStructure(t, agg, "60000ms", func(aggMap map[string]interface{}) {
		_, ok := aggMap["aggregations"].(map[string]interface{})
		assert.True(t, ok)
	})
}

func TestBuildCallRateAggregation(t *testing.T) {
	qb := setupTestQB()
	params := metricstore.BaseQueryParameters{
		Step: &commonStep,
	}

	agg := qb.BuildCallRateAggQuery(params, commonTimeRange)
	assert.NotNil(t, agg)

	testAggregationStructure(t, agg, "60000ms", func(aggMap map[string]interface{}) {
		assert.NotNil(t, aggMap["aggregations"])
	})
}

func TestBuildTimeSeriesAggQuery(t *testing.T) {
	qb := setupTestQB()
	params := metricstore.BaseQueryParameters{
		Step:             &commonStep,
		GroupByOperation: false,
	}
	subAgg := elastic.NewCumulativeSumAggregation()

	agg := qb.buildTimeSeriesAggQuery(params, commonTimeRange, "test_sub_agg", subAgg)
	assert.NotNil(t, agg)

	testAggregationStructure(t, agg, "60000ms", func(aggMap map[string]interface{}) {
		aggs := aggMap["aggregations"].(map[string]interface{})
		assert.NotNil(t, aggs["test_sub_agg"])
	})
}

func TestExecute(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	qb := NewQueryBuilder(client, *cfg)

	boolQuery := elastic.NewBoolQuery()
	aggQuery := elastic.NewDateHistogramAggregation().Field("startTimeMillis").FixedInterval("60000ms")

	result, err := qb.Execute(context.Background(), *boolQuery, aggQuery)

	assert.NoError(t, err)
	assert.NotNil(t, result)
}
