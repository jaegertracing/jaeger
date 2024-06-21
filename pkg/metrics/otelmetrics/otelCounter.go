package otelmetrics

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type otelCounter struct {
	counter      metric.Int64Counter
	attributeSet attribute.Set
}

func newOtelCounter(name string, tags map[string]string) *otelCounter {
	meter := otel.Meter("jaeger-V2")
	counter, err := meter.Int64Counter(name)
	if err != nil {
		panic(err)
	}

	attributes := make([]attribute.KeyValue, 0, len(tags))
	for k, v := range tags {
		attributes = append(attributes, attribute.String(k, v))
	}
	attributeSet := attribute.NewSet(attributes...)

	return &otelCounter{
		counter:      counter,
		attributeSet: attributeSet,
	}
}

func (c *otelCounter) Inc(value int64) {
	c.counter.Add(context.Background(), value, metric.WithAttributeSet(c.attributeSet))
}
