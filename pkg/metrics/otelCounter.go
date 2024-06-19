package metrics

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// otelCounter is a wrapper around otel.Counter

type otelCounter struct {
	counter   metric.Int64Counter
	ctx       context.Context
	attrs     []metric.AddOption
}

func (c *otelCounter) Inc(value int64) {
	c.counter.Add(c.ctx, value, c.attrs...)
}

func getAttributes(tags map[string]string) []metric.AddOption {
	var options []metric.AddOption
	for k, v := range tags {
		options = append(options, metric.WithAttributes(attribute.String(k, v)))
	}
	return options
}
