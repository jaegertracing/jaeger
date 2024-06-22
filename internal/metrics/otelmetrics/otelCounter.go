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
