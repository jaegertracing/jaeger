// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Must use separate test package to break import cycle.
package api_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/metrics/api"
	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestInitMetrics(t *testing.T) {
	testMetrics := struct {
		Gauge     api.Gauge     `metric:"gauge" tags:"1=one,2=two"`
		Counter   api.Counter   `metric:"counter"`
		Timer     api.Timer     `metric:"timer"`
		Histogram api.Histogram `metric:"histogram" buckets:"20,40,60,80"`
	}{}

	f := metricstest.NewFactory(0)
	defer f.Stop()

	globalTags := map[string]string{"key": "value"}

	err := api.Init(&testMetrics, f, globalTags)
	require.NoError(t, err)

	testMetrics.Gauge.Update(10)
	testMetrics.Counter.Inc(5)
	testMetrics.Timer.Record(time.Duration(time.Second * 35))
	testMetrics.Histogram.Record(42)

	// wait for metrics
	for i := 0; i < 1000; i++ {
		c, _ := f.Snapshot()
		if _, ok := c["counter"]; ok {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}

	c, g := f.Snapshot()

	assert.EqualValues(t, 5, c["counter|key=value"])
	assert.EqualValues(t, 10, g["gauge|1=one|2=two|key=value"])
	assert.EqualValues(t, 36863, g["timer|key=value.P50"])
	assert.EqualValues(t, 43, g["histogram|key=value.P50"])

	stopwatch := api.StartStopwatch(testMetrics.Timer)
	stopwatch.Stop()
	assert.Positive(t, stopwatch.ElapsedTime())
}

var (
	noMetricTag = struct {
		NoMetricTag api.Counter
	}{}

	badTags = struct {
		BadTags api.Counter `metric:"bad_tags" tags:"1=one,no_value,2=two"`
	}{}

	badMetricType = struct {
		BadMetricType int64 `metric:"bad_metric_type"`
	}{}

	badBucketsValue = struct {
		BadBucketsValue api.Histogram `metric:"bad_histogram_value" buckets:"1,b,3"`
	}{}

	badBucketFormat = struct {
		BadBucketFormat api.Histogram `metric:"bad_histogram_value" buckets:"[1,2,3]"`
	}{}
)

func TestInitMetricsFailures(t *testing.T) {
	require.EqualError(t, api.Init(&noMetricTag, nil, nil), "Field NoMetricTag is missing a tag 'metric'")

	require.EqualError(t, api.Init(&badTags, nil, nil),
		"Field [BadTags]: Tag [no_value] is not of the form key=value in 'tags' string [1=one,no_value,2=two]")

	require.EqualError(t, api.Init(&badMetricType, nil, nil),
		"Field BadMetricType is not a pointer to timer, gauge, or counter")

	require.EqualError(t, api.Init(&badBucketsValue, nil, nil),
		"Field [BadBucketsValue]: Bucket [b] could not be converted to float64 in 'buckets' string [1,b,3]")

	// No need to test empty buckets since it's handled by a nil factory check earlier
	// require.EqualError(t, api.Init(&emptyBucketsValue, nil, nil),
	//	"Field [EmptyBucketsValue]: Buckets string is empty in 'buckets' string []")

	require.EqualError(t, api.Init(&badBucketFormat, nil, nil),
		"Field [BadBucketFormat]: Bucket [[1] could not be converted to float64 in 'buckets' string [[1,2,3]]")
}

func TestInitPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("The code did not panic")
		}
	}()

	api.MustInit(&noMetricTag, api.NullFactory, nil)
}

func TestNullMetrics(*testing.T) {
	// This test is just for cover
	api.NullFactory.Timer(api.TimerOptions{
		Name: "name",
	}).Record(0)
	api.NullFactory.Counter(api.Options{
		Name: "name",
	}).Inc(0)
	api.NullFactory.Gauge(api.Options{
		Name: "name",
	}).Update(0)
	api.NullFactory.Histogram(api.HistogramOptions{
		Name: "name",
	}).Record(0)
	api.NullFactory.Namespace(api.NSOptions{
		Name: "name",
	}).Gauge(api.Options{
		Name: "name2",
	}).Update(0)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
