// Copyright (c) 2022 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"time"
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
