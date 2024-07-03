// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package otelmetrics

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/metric"
)

type otelTimer struct {
	histogram metric.Float64Histogram
	fixedCtx  context.Context
	option    metric.RecordOption
}

func (t *otelTimer) Record(d time.Duration) {
	t.histogram.Record(t.fixedCtx, d.Seconds(), t.option)
}
