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

package expvar

import (
	jlibmetrics "github.com/uber/jaeger-lib/metrics"

	"github.com/jaegertracing/jaeger/pkg/metrics"
)

// adapter is temporary type used to bridge metrics API in this package
// with that of jaeger-lib.
type adapter struct {
	f jlibmetrics.Factory
}

var _ metrics.Factory = (*adapter)(nil)

func newAdapter(f jlibmetrics.Factory) *adapter {
	return &adapter{f: f}
}

// Counter creates a Counter.
func (a *adapter) Counter(opts metrics.Options) metrics.Counter {
	return a.f.Counter(jlibmetrics.Options{
		Name: opts.Name,
		Tags: opts.Tags,
		Help: opts.Help,
	})
}

// Timer creates a Timer.
func (a *adapter) Timer(opts metrics.TimerOptions) metrics.Timer {
	return a.f.Timer(jlibmetrics.TimerOptions{
		Name:    opts.Name,
		Tags:    opts.Tags,
		Help:    opts.Help,
		Buckets: opts.Buckets,
	})
}

// Gauge creates a Gauge.
func (a *adapter) Gauge(opts metrics.Options) metrics.Gauge {
	return a.f.Gauge(jlibmetrics.Options{
		Name: opts.Name,
		Tags: opts.Tags,
		Help: opts.Help,
	})
}

// Histogram creates a Histogram.
func (a *adapter) Histogram(opts metrics.HistogramOptions) metrics.Histogram {
	return a.f.Histogram(jlibmetrics.HistogramOptions{
		Name:    opts.Name,
		Tags:    opts.Tags,
		Help:    opts.Help,
		Buckets: opts.Buckets,
	})
}

// Namespace creates a Namespace.
func (a *adapter) Namespace(opts metrics.NSOptions) metrics.Factory {
	return &adapter{f: a.f.Namespace(jlibmetrics.NSOptions{
		Name: opts.Name,
		Tags: opts.Tags,
	})}
}

// Unwrap returns underlying jaeger-lib factory.
func (a *adapter) Unwrap() jlibmetrics.Factory {
	return a.f
}
