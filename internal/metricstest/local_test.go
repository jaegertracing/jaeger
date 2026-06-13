// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package metricstest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/metrics"
)

func TestLocalMetrics(t *testing.T) {
	tags := map[string]string{
		"x": "y",
	}

	f := NewFactory(0)
	defer f.Stop()
	f.Counter(metrics.Options{
		Name: "my-counter",
		Tags: tags,
	}).Inc(4)
	f.Counter(metrics.Options{
		Name: "my-counter",
		Tags: tags,
	}).Inc(6)
	f.Counter(metrics.Options{
		Name: "my-counter",
	}).Inc(6)
	f.Counter(metrics.Options{
		Name: "other-counter",
	}).Inc(8)
	f.Gauge(metrics.Options{
		Name: "my-gauge",
	}).Update(25)
	f.Gauge(metrics.Options{
		Name: "my-gauge",
	}).Update(43)
	f.Gauge(metrics.Options{
		Name: "other-gauge",
	}).Update(74)
	f.Namespace(metrics.NSOptions{
		Name: "namespace",
		Tags: tags,
	}).Counter(metrics.Options{
		Name: "my-counter",
	}).Inc(7)
	f.Namespace(metrics.NSOptions{
		Name: "ns.subns",
	}).Counter(metrics.Options{
		Tags: map[string]string{"service": "a-service"},
	}).Inc(9)

	timings := map[string][]time.Duration{
		"foo-latency": {
			time.Second * 35,
			time.Second * 6,
			time.Millisecond * 576,
			time.Second * 12,
		},
		"bar-latency": {
			time.Minute*4 + time.Second*34,
			time.Minute*7 + time.Second*12,
			time.Second * 625,
			time.Second * 12,
		},
	}

	for metric, timing := range timings {
		for _, d := range timing {
			f.Timer(metrics.TimerOptions{
				Name: metric,
			}).Record(d)
		}
	}

	histogram := f.Histogram(metrics.HistogramOptions{
		Name: "my-histo",
	})
	histogram.Record(321)
	histogram.Record(42)

	c, g := f.Snapshot()
	require.NotNil(t, c)
	require.NotNil(t, g)

	assert.Equal(t, map[string]int64{
		"my-counter|x=y":             10,
		"my-counter":                 6,
		"other-counter":              8,
		"namespace.my-counter|x=y":   7,
		"ns.subns|service=a-service": 9,
	}, c)

	assert.Equal(t, map[string]int64{
		"bar-latency.P50":  274000,
		"bar-latency.P75":  432000,
		"bar-latency.P90":  432000,
		"bar-latency.P95":  432000,
		"bar-latency.P99":  432000,
		"bar-latency.P999": 432000,
		"foo-latency.P50":  6000,
		"foo-latency.P75":  12000,
		"foo-latency.P90":  12000,
		"foo-latency.P95":  12000,
		"foo-latency.P99":  12000,
		"foo-latency.P999": 12000,
		"my-gauge":         43,
		"my-histo.P50":     42,
		"my-histo.P75":     42,
		"my-histo.P90":     42,
		"my-histo.P95":     42,
		"my-histo.P99":     42,
		"my-histo.P999":    42,
		"other-gauge":      74,
	}, g)

	f.Clear()
	c, g = f.Snapshot()
	require.Empty(t, c)
	require.Empty(t, g)
}

func TestLocalMetricsInterval(t *testing.T) {
	f := NewFactory(time.Millisecond)
	defer f.Stop()

	f.Timer(metrics.TimerOptions{
		Name: "timer",
	}).Record(time.Millisecond * 100)

	f.tm.Lock()
	timer := f.timers["timer"]
	f.tm.Unlock()
	require.NotNil(t, timer)

	timer.Lock()
	assert.Len(t, timer.observations, 1)
	assert.Equal(t, int64(100), timer.observations[0])
	timer.Unlock()
}
