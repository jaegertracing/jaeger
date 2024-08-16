// Copyright (c) 2023 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package rpcmetrics

import (
	"sync"

	"github.com/jaegertracing/jaeger/pkg/metrics"
)

const (
	otherEndpointsPlaceholder = "other"
	endpointNameMetricTag     = "endpoint"
)

// Metrics is a collection of metrics for an endpoint describing
// throughput, success, errors, and performance.
type Metrics struct {
	// RequestCountSuccess is a counter of the total number of successes.
	RequestCountSuccess metrics.Counter `metric:"requests" tags:"error=false"`

	// RequestCountFailures is a counter of the number of times any failure has been observed.
	RequestCountFailures metrics.Counter `metric:"requests" tags:"error=true"`

	// RequestLatencySuccess is a latency histogram of successful requests.
	RequestLatencySuccess metrics.Timer `metric:"request_latency" tags:"error=false"`

	// RequestLatencyFailures is a latency histogram of failed requests.
	RequestLatencyFailures metrics.Timer `metric:"request_latency" tags:"error=true"`

	// HTTPStatusCode2xx is a counter of the total number of requests with HTTP status code 200-299
	HTTPStatusCode2xx metrics.Counter `metric:"http_requests" tags:"status_code=2xx"`

	// HTTPStatusCode3xx is a counter of the total number of requests with HTTP status code 300-399
	HTTPStatusCode3xx metrics.Counter `metric:"http_requests" tags:"status_code=3xx"`

	// HTTPStatusCode4xx is a counter of the total number of requests with HTTP status code 400-499
	HTTPStatusCode4xx metrics.Counter `metric:"http_requests" tags:"status_code=4xx"`

	// HTTPStatusCode5xx is a counter of the total number of requests with HTTP status code 500-599
	HTTPStatusCode5xx metrics.Counter `metric:"http_requests" tags:"status_code=5xx"`
}

func (m *Metrics) recordHTTPStatusCode(statusCode int64) {
	switch {
	case statusCode >= 200 && statusCode < 300:
		m.HTTPStatusCode2xx.Inc(1)
	case statusCode >= 300 && statusCode < 400:
		m.HTTPStatusCode3xx.Inc(1)
	case statusCode >= 400 && statusCode < 500:
		m.HTTPStatusCode4xx.Inc(1)
	case statusCode >= 500 && statusCode < 600:
		m.HTTPStatusCode5xx.Inc(1)
	}
}

// MetricsByEndpoint is a registry/cache of metrics for each unique endpoint name.
// Only maxNumberOfEndpoints Metrics are stored, all other endpoint names are mapped
// to a generic endpoint name "other".
type MetricsByEndpoint struct {
	metricsFactory    metrics.Factory
	endpoints         *normalizedEndpoints
	metricsByEndpoint map[string]*Metrics
	mux               sync.RWMutex
}

func newMetricsByEndpoint(
	metricsFactory metrics.Factory,
	normalizer NameNormalizer,
	maxNumberOfEndpoints int,
) *MetricsByEndpoint {
	return &MetricsByEndpoint{
		metricsFactory:    metricsFactory,
		endpoints:         newNormalizedEndpoints(maxNumberOfEndpoints, normalizer),
		metricsByEndpoint: make(map[string]*Metrics, maxNumberOfEndpoints+1), // +1 for "other"
	}
}

func (m *MetricsByEndpoint) get(endpoint string) *Metrics {
	safeName := m.endpoints.normalize(endpoint)
	if safeName == "" {
		safeName = otherEndpointsPlaceholder
	}
	m.mux.RLock()
	met := m.metricsByEndpoint[safeName]
	m.mux.RUnlock()
	if met != nil {
		return met
	}

	return m.getWithWriteLock(safeName)
}

// split to make easier to test
func (m *MetricsByEndpoint) getWithWriteLock(safeName string) *Metrics {
	m.mux.Lock()
	defer m.mux.Unlock()

	// it is possible that the name has been already registered after we released
	// the read lock and before we grabbed the write lock, so check for that.
	if met, ok := m.metricsByEndpoint[safeName]; ok {
		return met
	}

	// it would be nice to create the struct before locking, since Init() is somewhat
	// expensive, however some metrics backends (e.g. expvar) may not like duplicate metrics.
	met := &Metrics{}
	tags := map[string]string{endpointNameMetricTag: safeName}
	metrics.Init(met, m.metricsFactory, tags)

	m.metricsByEndpoint[safeName] = met
	return met
}
