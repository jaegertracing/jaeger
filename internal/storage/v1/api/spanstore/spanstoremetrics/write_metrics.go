// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstoremetrics

import (
	"time"

	"github.com/jaegertracing/jaeger/internal/metrics/api"
)

// WriteMetrics is a collection of metrics for write operations.
type WriteMetrics struct {
	Attempts   api.Counter `metric:"attempts"`
	Inserts    api.Counter `metric:"inserts"`
	Errors     api.Counter `metric:"errors"`
	LatencyOk  api.Timer   `metric:"latency-ok"`
	LatencyErr api.Timer   `metric:"latency-err"`
}

// NewWriter takes a metrics scope and creates a metrics struct
func NewWriter(factory api.Factory, tableName string) *WriteMetrics {
	t := &WriteMetrics{}
	api.Init(t, factory.Namespace(api.NSOptions{Name: tableName, Tags: nil}), nil)
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
