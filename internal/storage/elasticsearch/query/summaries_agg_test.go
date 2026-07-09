// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTermsQuerySource(t *testing.T) {
	src, err := NewTermsQuery("traceID", "1234567890abcdef").Source()
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"terms": map[string]any{"traceID": []any{"1234567890abcdef"}},
	}, src)
}

func TestExistsQuerySource(t *testing.T) {
	src, err := NewExistsQuery("parentSpanID").Source()
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"exists": map[string]any{"field": "parentSpanID"}}, src)
}

func TestMinAggregationSource(t *testing.T) {
	src, err := NewMinAggregation("startTime").Source()
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"min": map[string]any{"field": "startTime"}}, src)
}

func TestScriptedMaxAggregationSource(t *testing.T) {
	src, err := NewScriptedMaxAggregation("doc['startTime'].value + doc['duration'].value").Source()
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"max": map[string]any{
			"script": map[string]any{"source": "doc['startTime'].value + doc['duration'].value"},
		},
	}, src)
}

func TestFilterAggregationSourceWithAndWithoutSubAggs(t *testing.T) {
	// No sub-aggregations: just the filter (the error_count shape).
	src, err := NewFilterAggregation(NewExistsQuery("error")).Source()
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"filter": map[string]any{"exists": map[string]any{"field": "error"}},
	}, src)
	assert.NotContains(t, src.(map[string]any), "aggregations")

	// With a sub-aggregation (the root_span shape).
	src, err = NewFilterAggregation(NewBoolQuery().MustNot(NewExistsQuery("parentSpanID"))).
		SubAggregation("root_hit", NewTopHitsAggregation().Size(1)).
		Source()
	require.NoError(t, err)
	assert.Contains(t, src.(map[string]any), "aggregations")
}

func TestFilterAggregationPropagatesErrors(t *testing.T) {
	_, err := NewFilterAggregation(errQuery{}).Source()
	require.ErrorIs(t, err, errBadQuery)
	_, err = NewFilterAggregation(NewExistsQuery("a")).SubAggregation("x", errAgg{}).Source()
	require.ErrorIs(t, err, errBadQuery)
}

func TestTopHitsAggregationSource(t *testing.T) {
	src, err := NewTopHitsAggregation().
		Size(1).
		Sort("startTime", "asc").
		SourceIncludes("process.serviceName", "operationName").
		Source()
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"top_hits": map[string]any{
			"size":    1,
			"sort":    []any{map[string]any{"startTime": map[string]any{"order": "asc"}}},
			"_source": map[string]any{"includes": []string{"process.serviceName", "operationName"}},
		},
	}, src)
}
