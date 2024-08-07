// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package otelmetrics

import (
	"context"
	"log"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/jaegertracing/jaeger/pkg/metrics"
)

type otelFactory struct {
	meter      metric.Meter
	scope      string
	separator  string
	normalizer *strings.Replacer
	tags       map[string]string
}

func NewFactory(meterProvider metric.MeterProvider) metrics.Factory {
	return &otelFactory{
		meter:      meterProvider.Meter("jaeger-v2"),
		separator:  ".",
		normalizer: strings.NewReplacer(" ", "_", ".", "_", "-", "_"),
		tags:       make(map[string]string),
	}
}

func (f *otelFactory) Counter(opts metrics.Options) metrics.Counter {
	name := CounterNamingConvention(f.subScope(opts.Name))
	counter, err := f.meter.Int64Counter(name)
	if err != nil {
		log.Printf("Error creating OTEL counter: %v", err)
		return metrics.NullCounter
	}
	return &otelCounter{
		counter:  counter,
		fixedCtx: context.Background(),
		option:   attributeSetOption(f.mergeTags(opts.Tags)),
	}
}

func (f *otelFactory) Gauge(opts metrics.Options) metrics.Gauge {
	name := f.subScope(opts.Name)
	gauge, err := f.meter.Int64Gauge(name)
	if err != nil {
		log.Printf("Error creating OTEL gauge: %v", err)
		return metrics.NullGauge
	}

	return &otelGauge{
		gauge:    gauge,
		fixedCtx: context.Background(),
		option:   attributeSetOption(f.mergeTags(opts.Tags)),
	}
}

func (f *otelFactory) Histogram(opts metrics.HistogramOptions) metrics.Histogram {
	name := f.subScope(opts.Name)
	histogram, err := f.meter.Float64Histogram(name)
	if err != nil {
		log.Printf("Error creating OTEL histogram: %v", err)
		return metrics.NullHistogram
	}

	return &otelHistogram{
		histogram: histogram,
		fixedCtx:  context.Background(),
		option:    attributeSetOption(f.mergeTags(opts.Tags)),
	}
}

func (f *otelFactory) Timer(opts metrics.TimerOptions) metrics.Timer {
	name := f.subScope(opts.Name)
	timer, err := f.meter.Float64Histogram(name, metric.WithUnit("s"))
	if err != nil {
		log.Printf("Error creating OTEL timer: %v", err)
		return metrics.NullTimer
	}
	return &otelTimer{
		histogram: timer,
		fixedCtx:  context.Background(),
		option:    attributeSetOption(f.mergeTags(opts.Tags)),
	}
}

func (f *otelFactory) Namespace(opts metrics.NSOptions) metrics.Factory {
	return &otelFactory{
		meter:      f.meter,
		scope:      f.subScope(opts.Name),
		separator:  f.separator,
		normalizer: f.normalizer,
		tags:       f.mergeTags(opts.Tags),
	}
}

func (f *otelFactory) subScope(name string) string {
	if f.scope == "" {
		return f.normalize(name)
	}
	if name == "" {
		return f.normalize(f.scope)
	}
	return f.normalize(f.scope + f.separator + name)
}

func (f *otelFactory) normalize(v string) string {
	return f.normalizer.Replace(v)
}

func (f *otelFactory) mergeTags(tags map[string]string) map[string]string {
	merged := make(map[string]string)
	for k, v := range f.tags {
		merged[k] = v
	}
	for k, v := range tags {
		merged[k] = v
	}
	return merged
}

func attributeSetOption(tags map[string]string) metric.MeasurementOption {
	attributes := make([]attribute.KeyValue, 0, len(tags))
	for k, v := range tags {
		attributes = append(attributes, attribute.String(k, v))
	}
	return metric.WithAttributes(attributes...)
}

func CounterNamingConvention(name string) string {
	if !strings.HasSuffix(name, "_total") {
		name += "_total"
	}
	return name
}
