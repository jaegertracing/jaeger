// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package otelmetrics

import (
	"context"
	"log"

	"github.com/jaegertracing/jaeger/pkg/metrics"
	"go.opentelemetry.io/otel/attribute"
	metric "go.opentelemetry.io/otel/metric"
)

type otelFactory struct {
	meter metric.Meter
}

func NewFactory(meterProvider metric.MeterProvider) metrics.Factory {
	return &otelFactory{
		meter: meterProvider.Meter("jaeger-v2"),
	}
}

func (f *otelFactory) Counter(opts metrics.Options) metrics.Counter {
	counter, err := f.meter.Int64Counter(opts.Name)
	if err != nil {
		log.Printf("Error creating OTEL counter: %v", err)
		return metrics.NullCounter
	}

	attributes := make([]attribute.KeyValue, 0, len(opts.Tags))
	for k, v := range opts.Tags {
		attributes = append(attributes, attribute.String(k, v))
	}
	attributeSet := attribute.NewSet(attributes...)

	return &otelCounter{
		counter:  counter,
		fixedCtx: context.Background(),
		option:   metric.WithAttributeSet(attributeSet),
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
