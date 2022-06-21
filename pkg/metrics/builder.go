// Copyright (c) 2019 The Jaeger Authors.
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
	"errors"
	"expvar"
	"flag"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
	"github.com/uber/jaeger-lib/metrics"
	jexpvar "github.com/uber/jaeger-lib/metrics/expvar"
	jprom "github.com/uber/jaeger-lib/metrics/prometheus"
)

const (
	metricsBackend        = "metrics-backend"
	metricsHTTPRoute      = "metrics-http-route"
	defaultMetricsBackend = "prometheus"
	defaultMetricsRoute   = "/metrics"
)

var errUnknownBackend = errors.New("unknown metrics backend specified")

// Builder provides command line options to configure metrics backend used by Jaeger executables.
type Builder struct {
	Backend   string
	HTTPRoute string // endpoint name to expose metrics, e.g. for scraping
	handler   http.Handler
}

// AddFlags adds flags for Builder.
func AddFlags(flags *flag.FlagSet) {
	flags.String(
		metricsBackend,
		defaultMetricsBackend,
		"Defines which metrics backend to use for metrics reporting: expvar, prometheus, none")
	flags.String(
		metricsHTTPRoute,
		defaultMetricsRoute,
		"Defines the route of HTTP endpoint for metrics backends that support scraping")
}

// InitFromViper initializes Builder with properties retrieved from Viper.
func (b *Builder) InitFromViper(v *viper.Viper) *Builder {
	b.Backend = v.GetString(metricsBackend)
	b.HTTPRoute = v.GetString(metricsHTTPRoute)
	return b
}

// CreateMetricsFactory creates a metrics factory based on the configured type of the backend.
// If the metrics backend supports HTTP endpoint for scraping, it is stored in the builder and
// can be later added by RegisterHandler function.
func (b *Builder) CreateMetricsFactory(namespace string) (Factory, error) {
	if b.Backend == "prometheus" {
		metricsFactory := jprom.New().Namespace(metrics.NSOptions{Name: namespace, Tags: nil})
		b.handler = promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{DisableCompression: true})
		return &adapter{f: metricsFactory}, nil
	}
	if b.Backend == "expvar" {
		metricsFactory := jexpvar.NewFactory(10).Namespace(metrics.NSOptions{Name: namespace, Tags: nil})
		b.handler = expvar.Handler()
		return &adapter{f: metricsFactory}, nil
	}
	if b.Backend == "none" || b.Backend == "" {
		return &adapter{f: metrics.NullFactory}, nil
	}
	return nil, errUnknownBackend
}

// Handler returns an http.Handler for the metrics endpoint.
func (b *Builder) Handler() http.Handler {
	return b.handler
}

type adapter struct {
	f metrics.Factory
}

func (a *adapter) Counter(opts Options) Counter {
	return a.f.Counter(metrics.Options{
		Name: opts.Name,
		Tags: opts.Tags,
		Help: opts.Help,
	})
}

func (a *adapter) Timer(opts TimerOptions) Timer {
	return a.f.Timer(metrics.TimerOptions{
		Name:    opts.Name,
		Tags:    opts.Tags,
		Help:    opts.Help,
		Buckets: opts.Buckets,
	})
}

func (a *adapter) Gauge(opts Options) Gauge {
	return a.f.Gauge(metrics.Options{
		Name: opts.Name,
		Tags: opts.Tags,
		Help: opts.Help,
	})
}

func (a *adapter) Histogram(opts HistogramOptions) Histogram {
	return a.f.Histogram(metrics.HistogramOptions{
		Name:    opts.Name,
		Tags:    opts.Tags,
		Help:    opts.Help,
		Buckets: opts.Buckets,
	})
}

func (a *adapter) Namespace(opts NSOptions) Factory {
	return &adapter{f: a.f.Namespace(metrics.NSOptions{
		Name: opts.Name,
		Tags: opts.Tags,
	})}
}
