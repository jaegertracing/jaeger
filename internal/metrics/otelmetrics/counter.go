// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package otelmetrics

import (
	"context"

	"go.opentelemetry.io/otel/metric"
)

type otelCounter struct {
	counter  metric.Int64Counter
	fixedCtx context.Context
	option   metric.AddOption
}

func (c *otelCounter) Inc(value int64) {
	c.counter.Add(c.fixedCtx, value, c.option)
}
