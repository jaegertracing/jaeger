// Copyright (c) 2022 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metrics

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// NSOptions defines the name and tags map associated with a factory namespace
type NSOptions struct {
	Name string
	Tags map[string]string
}

// Options defines the information associated with a metric
type Options struct {
	Name string
	Tags map[string]string
	Help string
}

// TimerOptions defines the information associated with a metric
type TimerOptions struct {
	Name    string
	Tags    map[string]string
	Help    string
	Buckets []time.Duration
}

// HistogramOptions defines the information associated with a metric
type HistogramOptions struct {
	Name    string
	Tags    map[string]string
	Help    string
	Buckets []float64
}

type otelFactory struct {
	meter metric.Meter
}

func NewOTelFactory() Factory {
	return &otelFactory{
		meter: otel.Meter("jaeger-V2"),
	}
}

func (f *otelFactory) Counter(options Options) Counter {
	counter, _ := f.meter.Int64Counter(options.Name)
	attrs := getAttributes(options.Tags)
	return &otelCounter{
		counter: counter,
		ctx:     context.Background(),
		attrs:   attrs,
	}
}

func (f *otelFactory) Timer(options TimerOptions) Timer {
	// Implement the OTEL Timer
	return NullTimer
}

func (f *otelFactory) Gauge(options Options) Gauge {
	// Implement the OTEL Gauge
	return NullGauge
}

func (f *otelFactory) Histogram(options HistogramOptions) Histogram {
	// Implement the OTEL Histogram
	return NullHistogram
}

func (f *otelFactory) Namespace(scope NSOptions) Factory {
	return &otelFactory{
		meter: otel.Meter(scope.Name),
	}
}

// Factory creates new metrics
type Factory interface {
	Counter(metric Options) Counter
	Timer(metric TimerOptions) Timer
	Gauge(metric Options) Gauge
	Histogram(metric HistogramOptions) Histogram

	// Namespace returns a nested metrics factory.
	Namespace(scope NSOptions) Factory
}

// NullFactory is a metrics factory that returns NullCounter, NullTimer, and NullGauge.
var NullFactory Factory = nullFactory{}

type nullFactory struct{}

func (nullFactory) Counter(Options) Counter {
	return NullCounter
}

func (nullFactory) Timer(TimerOptions) Timer {
	return NullTimer
}

func (nullFactory) Gauge(Options) Gauge {
	return NullGauge
}

func (nullFactory) Histogram(HistogramOptions) Histogram {
	return NullHistogram
}
func (nullFactory) Namespace(NSOptions /* scope */) Factory { return NullFactory }
