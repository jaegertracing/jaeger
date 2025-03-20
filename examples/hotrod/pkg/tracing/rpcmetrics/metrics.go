// Copyright (c) 2023 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package rpcmetrics

import (
	"net/http"
	"sync"

	"github.com/jaegertracing/jaeger/internal/metrics/api"
)

const (
	otherEndpointsPlaceholder = "other"
	endpointNameMetricTag     = "endpoint"
)

// Metrics is a collection of metrics for an endpoint describing
// throughput, success, errors, and performance.
type Metrics struct {
	// RequestCountSuccess is a counter of the total number of successes.
	RequestCountSuccess api.Counter `metric:"requests" tags:"error=false"`

	// RequestCountFailures is a counter of the number of times any failure has been observed.
	RequestCountFailures api.Counter `metric:"requests" tags:"error=true"`

	// RequestLatencySuccess is a latency histogram of successful requests.
	RequestLatencySuccess api.Timer `metric:"request_latency" tags:"error=false"`

	// RequestLatencyFailures is a latency histogram of failed requests.
	RequestLatencyFailures api.Timer `metric:"request_latency" tags:"error=true"`

	// HTTPStatusCode2xx is a counter of the total number of requests with HTTP status code 200-299
	HTTPStatusCode2xx api.Counter `metric:"http_requests" tags:"status_code=2xx"`

	// HTTPStatusCode3xx is a counter of the total number of requests with HTTP status code 300-399
	HTTPStatusCode3xx api.Counter `metric:"http_requests" tags:"status_code=3xx"`

	// HTTPStatusCode4xx is a counter of the total number of requests with HTTP status code 400-499
	HTTPStatusCode4xx api.Counter `metric:"http_requests" tags:"status_code=4xx"`

	// HTTPStatusCode5xx is a counter of the total number of requests with HTTP status code 500-599
	HTTPStatusCode5xx api.Counter `metric:"http_requests" tags:"status_code=5xx"`
}

func (m *Metrics) recordHTTPStatusCode(statusCode int64) {
	switch {
	case statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices:
		m.HTTPStatusCode2xx.Inc(1)
	case statusCode >= http.StatusMultipleChoices && statusCode < http.StatusBadRequest:
		m.HTTPStatusCode3xx.Inc(1)
	case statusCode >= http.StatusBadRequest && statusCode < http.StatusInternalServerError:
		m.HTTPStatusCode4xx.Inc(1)
	case statusCode >= http.StatusInternalServerError && statusCode < 600:
		m.HTTPStatusCode5xx.Inc(1)
	}
}

// MetricsByEndpoint is a registry/cache of metrics for each unique endpoint name.
// Only maxNumberOfEndpoints Metrics are stored, all other endpoint names are mapped
// to a generic endpoint name "other".
type MetricsByEndpoint struct {
	metricsFactory    api.Factory
	endpoints         *normalizedEndpoints
	metricsByEndpoint map[string]*Metrics
	mux               sync.RWMutex
}

func newMetricsByEndpoint(
	metricsFactory api.Factory,
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
	api.Init(met, m.metricsFactory, tags)

	m.metricsByEndpoint[safeName] = met
	return met
}
