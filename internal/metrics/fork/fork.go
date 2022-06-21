// Copyright (c) 2020 The Jaeger Authors.
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

package fork

import "github.com/jaegertracing/jaeger/pkg/metrics"

// Factory represents a metrics factory that delegates metrics with
// forkNamespace to forkFactory otherwise - defaultFactory is used.
type Factory struct {
	forkNamespace  string
	forkFactory    metrics.Factory
	defaultFactory metrics.Factory
}

// New creates new fork.Factory.
func New(forkNamespace string, forkFactory, defaultFactory metrics.Factory) metrics.Factory {
	return &Factory{
		forkNamespace:  forkNamespace,
		forkFactory:    forkFactory,
		defaultFactory: defaultFactory,
	}
}

// Gauge implements metrics.Factory interface.
func (f *Factory) Gauge(options metrics.Options) metrics.Gauge {
	return f.defaultFactory.Gauge(options)
}

// Counter implements metrics.Factory interface.
func (f *Factory) Counter(metric metrics.Options) metrics.Counter {
	return f.defaultFactory.Counter(metric)
}

// Timer implements metrics.Factory interface.
func (f *Factory) Timer(metric metrics.TimerOptions) metrics.Timer {
	return f.defaultFactory.Timer(metric)
}

// Histogram implements metrics.Factory interface.
func (f *Factory) Histogram(metric metrics.HistogramOptions) metrics.Histogram {
	return f.defaultFactory.Histogram(metric)
}

// Namespace implements metrics.Factory interface.
func (f *Factory) Namespace(scope metrics.NSOptions) metrics.Factory {
	if scope.Name == f.forkNamespace {
		return f.forkFactory.Namespace(scope)
	}

	return &Factory{
		forkNamespace:  f.forkNamespace,
		forkFactory:    f.forkFactory.Namespace(scope),
		defaultFactory: f.defaultFactory.Namespace(scope),
	}
}
