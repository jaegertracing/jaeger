// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDateHistogramAndMetricAccessors parses the metricstore's call-rate shape: a
// date_histogram whose numeric-keyed buckets each carry a cumulative_sum metric.
func TestDateHistogramAndMetricAccessors(t *testing.T) {
	aggs := Aggregations{
		"results_buckets": json.RawMessage(`{"buckets":[
			{"key":1577930045000,"key_as_string":"2020-01-02T02:34:05Z","doc_count":5,"cumulative_requests":{"value":1.5}},
			{"key":1577930105000,"doc_count":0}
		]}`),
	}

	hist, ok := aggs.DateHistogram("results_buckets")
	require.True(t, ok)
	require.Len(t, hist.Buckets, 2)

	assert.Equal(t, int64(1577930045000), hist.Buckets[0].Key)
	assert.Equal(t, 5, hist.Buckets[0].DocCount)
	value, ok := hist.Buckets[0].Metric("cumulative_requests")
	require.True(t, ok)
	require.NotNil(t, value)
	assert.InDelta(t, 1.5, *value, 1e-9)

	// The empty bucket keeps its numeric key and has no metric.
	assert.Equal(t, int64(1577930105000), hist.Buckets[1].Key)
	_, ok = hist.Buckets[1].Metric("cumulative_requests")
	assert.False(t, ok)
}

// TestPercentilesAccessor parses the metricstore's latency shape: a percentiles
// sub-aggregation keyed by percentile rank.
func TestPercentilesAccessor(t *testing.T) {
	aggs := Aggregations{
		"percentiles_of_bucket": json.RawMessage(`{"values":{"95.0":12.3,"99.0":45.6}}`),
	}
	p, ok := aggs.Percentiles("percentiles_of_bucket")
	require.True(t, ok)
	assert.InDelta(t, 12.3, p.Values["95.0"], 1e-9)
	assert.InDelta(t, 45.6, p.Values["99.0"], 1e-9)
}

func TestDateHistogramAndPercentilesMissingOrMalformed(t *testing.T) {
	aggs := Aggregations{"bad": json.RawMessage(`not json`)}

	_, ok := aggs.DateHistogram("absent")
	assert.False(t, ok)
	_, ok = aggs.DateHistogram("bad")
	assert.False(t, ok)
	_, ok = aggs.Percentiles("absent")
	assert.False(t, ok)
	_, ok = aggs.Percentiles("bad")
	assert.False(t, ok)
}

// TestHistogramBucketRoundTrip covers HistogramBucket's Marshal/Unmarshal: a bucket
// marshalled to wire JSON and back preserves its key, count, and sub-aggregations.
func TestHistogramBucketRoundTrip(t *testing.T) {
	original := HistogramBucket{
		Key:      1577930045000,
		DocCount: 7,
		Aggregations: Aggregations{
			"cumulative_requests": json.RawMessage(`{"value":2.5}`),
		},
	}
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var got HistogramBucket
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, original.Key, got.Key)
	assert.Equal(t, original.DocCount, got.DocCount)
	value, ok := got.Metric("cumulative_requests")
	require.True(t, ok)
	assert.InDelta(t, 2.5, *value, 1e-9)
}

// TestAggregationBucketRoundTrip covers AggregationBucket.MarshalJSON: a terms
// bucket marshalled to wire JSON and back preserves its string key and count.
func TestAggregationBucketRoundTrip(t *testing.T) {
	original := AggregationBucket{Key: "operationA", DocCount: 3}
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var got AggregationBucket
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, "operationA", got.Key)
	assert.Equal(t, 3, got.DocCount)
}

// TestMarshalBucketMalformedSubAgg covers marshalBucket's error path: a malformed
// raw sub-aggregation makes json.Marshal fail.
func TestMarshalBucketMalformedSubAgg(t *testing.T) {
	b := HistogramBucket{
		Key:          1,
		Aggregations: Aggregations{"broken": json.RawMessage(`not json`)},
	}
	_, err := json.Marshal(b)
	require.Error(t, err)
}

func TestHistogramBucketUnmarshalErrors(t *testing.T) {
	// Non-numeric key fails the decode.
	var b HistogramBucket
	require.Error(t, json.Unmarshal([]byte(`{"key":"not-a-number","doc_count":1}`), &b))
	// Non-integer doc_count fails the decode.
	require.Error(t, json.Unmarshal([]byte(`{"key":1,"doc_count":"x"}`), &b))
	// A non-object fails the decode.
	require.Error(t, json.Unmarshal([]byte(`5`), &b))
}
