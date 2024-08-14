// Copyright (c) 2023 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package rpcmetrics

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/internal/metricstest"
)

// E.g. tags("key", "value", "key", "value")
func tags(kv ...string) map[string]string {
	m := make(map[string]string)
	for i := 0; i < len(kv)-1; i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return m
}

func endpointTags(endpoint string, kv ...string) map[string]string {
	return tags(append([]string{"endpoint", endpoint}, kv...)...)
}

func TestMetricsByEndpoint(t *testing.T) {
	met := metricstest.NewFactory(0)
	mbe := newMetricsByEndpoint(met, DefaultNameNormalizer, 2)

	m1 := mbe.get("abc1")
	m2 := mbe.get("abc1")               // from cache
	m2a := mbe.getWithWriteLock("abc1") // from cache in double-checked lock
	assert.Equal(t, m1, m2)
	assert.Equal(t, m1, m2a)

	m3 := mbe.get("abc3")
	m4 := mbe.get("overflow")
	m5 := mbe.get("overflow2")

	for _, m := range []*Metrics{m1, m2, m2a, m3, m4, m5} {
		m.RequestCountSuccess.Inc(1)
	}

	met.AssertCounterMetrics(t,
		metricstest.ExpectedMetric{Name: "requests", Tags: endpointTags("abc1", "error", "false"), Value: 3},
		metricstest.ExpectedMetric{Name: "requests", Tags: endpointTags("abc3", "error", "false"), Value: 1},
		metricstest.ExpectedMetric{Name: "requests", Tags: endpointTags("other", "error", "false"), Value: 2},
	)
}
