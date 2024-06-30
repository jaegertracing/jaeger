// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package otelmetrics

import (
	"context"

	"go.opentelemetry.io/otel/metric"
)

type otelHistogram struct {
	histogram metric.Float64Histogram
	fixedCtx  context.Context
	option    metric.RecordOption
}

func (h *otelHistogram) Record(value float64) {
	h.histogram.Record(h.fixedCtx, value, h.option)
}
