// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package otelmetrics

import (
	"context"
	"log"

	"go.opentelemetry.io/otel/attribute"
	metric "go.opentelemetry.io/otel/metric"

	"github.com/jaegertracing/jaeger/pkg/metrics"
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
	return &otelCounter{
		counter:  counter,
		fixedCtx: context.Background(),
		option:   attributeSetOption(opts.Tags),
	}
}

func (f *otelFactory) Gauge(opts metrics.Options) metrics.Gauge {
	gauge, err := f.meter.Int64Gauge(opts.Name)
	if err != nil {
		log.Printf("Error creating OTEL gauge: %v", err)
		return metrics.NullGauge

	}

	return &otelGauge{
		gauge:    gauge,
		fixedCtx: context.Background(),
		option:   attributeSetOption(opts.Tags),
	}
}

func (f *otelFactory) Histogram(opts metrics.HistogramOptions) metrics.Histogram {
	histogram, err := f.meter.Float64Histogram(opts.Name)
	if err != nil {
		log.Printf("Error creating OTEL histogram: %v", err)
		return metrics.NullHistogram
	}

	return &otelHistogram{
		histogram: histogram,
		fixedCtx:  context.Background(),
		option:    attributeSetOption(opts.Tags),
	}
}

func (f *otelFactory) Timer(opts metrics.TimerOptions) metrics.Timer {
	timer, err := f.meter.Float64Histogram(opts.Name)
	if err != nil {
		log.Printf("Error creating OTEL timer: %v", err)
		return metrics.NullTimer
	}
	return &otelTimer{
		histogram: timer,
		fixedCtx:  context.Background(),
		option:    attributeSetOption(opts.Tags),
	}
}

func (f *otelFactory) Namespace(opts metrics.NSOptions) metrics.Factory {
	return f
}

func attributeSetOption(tags map[string]string) metric.MeasurementOption {
	attributes := make([]attribute.KeyValue, 0, len(tags))
	for k, v := range tags {
		attributes = append(attributes, attribute.String(k, v))
	}
	return metric.WithAttributes(attributes...)
}
