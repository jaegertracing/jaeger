package otelmetrics

import (
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type otelFactory struct{}

func NewOTELFactory() metrics.Factory {
	return &otelFactory{}
}

func (f *otelFactory) Counter(opts metrics.Options) metrics.Counter {
	meter := otel.Meter("jaeger-V2")
	counter, err := meter.Int64Counter(opts.Name)
	if err != nil {
		panic(err)
	}

	attributes := make([]attribute.KeyValue, 0, len(opts.Tags))
	for k, v := range opts.Tags {
		attributes = append(attributes, attribute.String(k, v))
	}
	attributeSet := attribute.NewSet(attributes...)

	return &otelCounter{
		counter:      counter,
		attributeSet: attributeSet,
	}
}

func (f *otelFactory) Gauge(opts metrics.Options) metrics.Gauge {
	// TODO: Implement OTEL Gauge
	return nil
}

func (f *otelFactory) Timer(opts metrics.TimerOptions) metrics.Timer {
	// TODO: Implement OTEL Timer
	return nil
}

func (f *otelFactory) Histogram(opts metrics.HistogramOptions) metrics.Histogram {
	// TODO: Implement OTEL Histogram
	return nil
}

func (f *otelFactory) Namespace(opts metrics.NSOptions) metrics.Factory {
	return f
}
