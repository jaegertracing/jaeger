package otelmetrics

import "github.com/jaegertracing/jaeger/pkg/metrics"

type otelFactory struct{}

func NewOTELFactory() metrics.Factory {
	return &otelFactory{}
}

func (f *otelFactory) Counter(opts metrics.Options) metrics.Counter {
	return newOtelCounter(opts.Name, opts.Tags)
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
