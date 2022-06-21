// Copyright (c) 2022 The Jaeger Authors.
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

package jlibadapter

import (
	jlibmetrics "github.com/uber/jaeger-lib/metrics"

	"github.com/jaegertracing/jaeger/pkg/metrics"
)

// adapter is temporary type used to bridge metrics API in this package
// with that of jaeger-lib.
type adapter struct {
	f metrics.Factory
}

var _ jlibmetrics.Factory = (*adapter)(nil)

// NewAdapter wraps internal metrics.Factory to look like jaeger-lib version.
func NewAdapter(f metrics.Factory) jlibmetrics.Factory {
	return &adapter{f: f}
}

// Counter creates a Counter.
func (a *adapter) Counter(opts jlibmetrics.Options) jlibmetrics.Counter {
	return a.f.Counter(metrics.Options{
		Name: opts.Name,
		Tags: opts.Tags,
		Help: opts.Help,
	})
}

// Timer creates a Timer.
func (a *adapter) Timer(opts jlibmetrics.TimerOptions) jlibmetrics.Timer {
	return a.f.Timer(metrics.TimerOptions{
		Name:    opts.Name,
		Tags:    opts.Tags,
		Help:    opts.Help,
		Buckets: opts.Buckets,
	})
}

// Gauge creates a Gauge.
func (a *adapter) Gauge(opts jlibmetrics.Options) jlibmetrics.Gauge {
	return a.f.Gauge(metrics.Options{
		Name: opts.Name,
		Tags: opts.Tags,
		Help: opts.Help,
	})
}

// Histogram creates a Histogram.
func (a *adapter) Histogram(opts jlibmetrics.HistogramOptions) jlibmetrics.Histogram {
	return a.f.Histogram(metrics.HistogramOptions{
		Name:    opts.Name,
		Tags:    opts.Tags,
		Help:    opts.Help,
		Buckets: opts.Buckets,
	})
}

// Namespace creates a Namespace.
func (a *adapter) Namespace(opts jlibmetrics.NSOptions) jlibmetrics.Factory {
	return &adapter{f: a.f.Namespace(metrics.NSOptions{
		Name: opts.Name,
		Tags: opts.Tags,
	})}
}
