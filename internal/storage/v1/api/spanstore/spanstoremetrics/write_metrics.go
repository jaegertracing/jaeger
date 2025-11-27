// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstoremetrics

import (
	"time"

	"github.com/jaegertracing/jaeger/internal/metrics"
)

// WriteMetrics is a collection of metrics for write operations.
type WriteMetrics struct {
	Attempts   metrics.Counter `metric:"attempts"`
	Inserts    metrics.Counter `metric:"inserts"`
	Errors     metrics.Counter `metric:"errors"`
	LatencyOk  metrics.Timer   `metric:"latency-ok" buckets:"5ms,10ms,25ms,50ms,100ms,250ms,500ms,1s,2s,5s,10s"`
	LatencyErr metrics.Timer   `metric:"latency-err" buckets:"5ms,10ms,25ms,50ms,100ms,250ms,500ms,1s,2s,5s,10s"`
}

// NewWriter takes a metrics scope and creates a metrics struct
func NewWriter(factory metrics.Factory, tableName string) *WriteMetrics {
	t := &WriteMetrics{}
	metrics.Init(t, factory.Namespace(metrics.NSOptions{Name: tableName, Tags: nil}), nil)
	return t
}

// Emit will record success or failure counts and latency metrics depending on the passed error.
func (t *WriteMetrics) Emit(err error, latency time.Duration) {
	t.Attempts.Inc(1)
	if err != nil {
		t.LatencyErr.Record(latency)
		t.Errors.Inc(1)
	} else {
		t.LatencyOk.Record(latency)
		t.Inserts.Inc(1)
	}
}
