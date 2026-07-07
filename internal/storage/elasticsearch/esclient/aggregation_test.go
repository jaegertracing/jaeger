// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAggregationBucketNestedAccessors decodes a trace-summaries-shaped bucket and
// walks its nested sub-aggregations through the typed accessors.
func TestAggregationBucketNestedAccessors(t *testing.T) {
	const raw = `{
		"key": "trace-1",
		"doc_count": 5,
		"min_start": {"value": 100},
		"max_end": {"value": 900},
		"error_count": {"doc_count": 2},
		"services": {"buckets": [
			{"key": "svc-a", "doc_count": 3, "service_errors": {"doc_count": 1}}
		]},
		"root_span": {"doc_count": 1, "root_hit": {"hits": {"hits": [
			{"_source": {"operationName": "op"}}
		]}}}
	}`
	var b AggregationBucket
	require.NoError(t, json.Unmarshal([]byte(raw), &b))

	assert.Equal(t, "trace-1", b.Key)
	assert.Equal(t, 5, b.DocCount)

	minStart, ok := b.Metric("min_start")
	require.True(t, ok)
	require.NotNil(t, minStart)
	assert.InDelta(t, 100.0, *minStart, 0)

	errCount, ok := b.Filter("error_count")
	require.True(t, ok)
	assert.Equal(t, 2, errCount.DocCount)

	services, ok := b.Terms("services")
	require.True(t, ok)
	require.Len(t, services.Buckets, 1)
	assert.Equal(t, "svc-a", services.Buckets[0].Key)
	svcErrs, ok := services.Buckets[0].Filter("service_errors")
	require.True(t, ok)
	assert.Equal(t, 1, svcErrs.DocCount)

	rootSpan, ok := b.Filter("root_span")
	require.True(t, ok)
	hits, ok := rootSpan.TopHits("root_hit")
	require.True(t, ok)
	require.Len(t, hits.Hits, 1)
	assert.JSONEq(t, `{"operationName":"op"}`, string(hits.Hits[0].Source))
}

func TestAggregationBucketMissingSubAggsReturnFalse(t *testing.T) {
	var b AggregationBucket
	require.NoError(t, json.Unmarshal([]byte(`{"key":"k","doc_count":1}`), &b))
	_, ok := b.Metric("absent")
	assert.False(t, ok)
	_, ok = b.Filter("absent")
	assert.False(t, ok)
	_, ok = b.Terms("absent")
	assert.False(t, ok)
	_, ok = b.TopHits("absent")
	assert.False(t, ok)
}

func TestAggregationBucketNullMetric(t *testing.T) {
	var b AggregationBucket
	require.NoError(t, json.Unmarshal([]byte(`{"key":"k","doc_count":0,"m":{"value":null}}`), &b))
	v, ok := b.Metric("m")
	require.True(t, ok)
	assert.Nil(t, v, "a null metric is present but has no value")
}

// TestAggregationBucketMalformedSubAggs feeds each accessor a sub-aggregation
// whose JSON has the wrong shape, exercising the decode-failure branches.
func TestAggregationBucketMalformedSubAggs(t *testing.T) {
	var b AggregationBucket
	require.NoError(t, json.Unmarshal([]byte(`{
		"key": "k", "doc_count": 1,
		"metric": "not-an-object",
		"filter": "not-an-object",
		"terms": 42,
		"top": [1,2]
	}`), &b))

	_, ok := b.Metric("metric")
	assert.False(t, ok)
	_, ok = b.Filter("filter")
	assert.False(t, ok)
	_, ok = b.Terms("terms")
	assert.False(t, ok)
	_, ok = b.TopHits("top")
	assert.False(t, ok)
}

func TestAggregationBucketUnmarshalErrors(t *testing.T) {
	var b AggregationBucket
	require.Error(t, json.Unmarshal([]byte(`"not-an-object"`), &b))
	require.Error(t, json.Unmarshal([]byte(`{"doc_count":"not-a-number"}`), &b))
}

func TestFilterResultUnmarshalError(t *testing.T) {
	var f FilterResult
	require.Error(t, json.Unmarshal([]byte(`123`), &f))
	require.Error(t, json.Unmarshal([]byte(`{"doc_count":"not-a-number"}`), &f))
}
