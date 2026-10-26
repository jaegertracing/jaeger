// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBoolQuerySingleClauseRendersAsObject(t *testing.T) {
	q := NewBoolQuery().Must(NewTermQuery("traceID", "abc"))
	src, err := q.Source()
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"bool": map[string]any{
			"must": map[string]any{"term": map[string]any{"traceID": "abc"}},
		},
	}, src)
}

func TestBoolQueryMultipleClausesRenderAsArray(t *testing.T) {
	q := NewBoolQuery().Must(
		NewTermQuery("traceID", "abc"),
		NewRangeQuery("startTimeMillis").Gte(1).Lte(2),
	)
	src, err := q.Source()
	require.NoError(t, err)
	must, ok := src.(map[string]any)["bool"].(map[string]any)["must"].([]any)
	require.True(t, ok, "must with >1 clause is an array")
	assert.Len(t, must, 2)
}

func TestBoolQueryShouldAndMustNot(t *testing.T) {
	q := NewBoolQuery().
		Should(NewTermQuery("a", 1)).
		MustNot(NewTermQuery("b", 2))
	src, err := q.Source()
	require.NoError(t, err)
	b := src.(map[string]any)["bool"].(map[string]any)
	assert.Equal(t, map[string]any{"term": map[string]any{"a": 1}}, b["should"])
	assert.Equal(t, map[string]any{"term": map[string]any{"b": 2}}, b["must_not"])
	assert.NotContains(t, b, "must", "an unused clause is omitted")
}

// TestBoolQueryFilterRendersAsArray reproduces the metricstore bool query: several
// filter clauses render as an array under "filter".
func TestBoolQueryFilterRendersAsArray(t *testing.T) {
	q := NewBoolQuery().Filter(
		NewTermsQuery("process.serviceName", "svc"),
		NewTermQuery("tag.error", true),
	)
	src, err := q.Source()
	require.NoError(t, err)
	b := src.(map[string]any)["bool"].(map[string]any)
	assert.Equal(t, []any{
		map[string]any{"terms": map[string]any{"process.serviceName": []any{"svc"}}},
		map[string]any{"term": map[string]any{"tag.error": true}},
	}, b["filter"])
	assert.NotContains(t, b, "must", "an unused clause is omitted")
}

func TestBoolQueryEmptyRendersEmptyObject(t *testing.T) {
	src, err := NewBoolQuery().Source()
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"bool": map[string]any{}}, src)
}

func TestBoolQueryPropagatesClauseError(t *testing.T) {
	_, err := NewBoolQuery().Must(errQuery{}).Source()
	require.ErrorIs(t, err, errBadQuery)
	// The array path (>1 clause) also propagates.
	_, err = NewBoolQuery().Must(NewTermQuery("a", 1), errQuery{}).Source()
	require.ErrorIs(t, err, errBadQuery)
}
