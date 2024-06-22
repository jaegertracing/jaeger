package otelmetrics

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type otelCounter struct {
	counter      metric.Int64Counter
	attributeSet attribute.Set
}

func (c *otelCounter) Inc(value int64) {
	c.counter.Add(context.Background(), value, metric.WithAttributeSet(c.attributeSet))
}
