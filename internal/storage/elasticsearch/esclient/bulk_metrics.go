// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"time"

	"github.com/jaegertracing/jaeger/internal/metrics"
)

// bulkLatencyFactory sets timer boundaries for bulk writes while leaving other
// storage-write timers on their existing boundaries.
type bulkLatencyFactory struct {
	metrics.Factory
}

func (f bulkLatencyFactory) Timer(opts metrics.TimerOptions) metrics.Timer {
	opts.Buckets = []time.Duration{
		5 * time.Millisecond,
		10 * time.Millisecond,
		25 * time.Millisecond,
		50 * time.Millisecond,
		100 * time.Millisecond,
		250 * time.Millisecond,
		500 * time.Millisecond,
		time.Second,
		2500 * time.Millisecond,
		5 * time.Second,
		10 * time.Second,
	}
	return f.Factory.Timer(opts)
}

func (f bulkLatencyFactory) Namespace(opts metrics.NSOptions) metrics.Factory {
	return bulkLatencyFactory{Factory: f.Factory.Namespace(opts)}
}
