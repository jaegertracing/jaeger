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
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
	xkit "github.com/uber/jaeger-lib/metrics/go-kit"
	kitexpvar "github.com/uber/jaeger-lib/metrics/go-kit/expvar"
	kitprom "github.com/uber/jaeger-lib/metrics/go-kit/prometheus"

	"github.com/uber/jaeger-lib/metrics"
)

const (
	metricsBackend        = "metrics-backend"
	metricsHTTPRoute      = "metrics-http-route"
	defaultMetricsBackend = "expvar"
	defaultMetricsRoute   = "/debug/vars"
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
		fmt.Sprintf("Defines which metrics backend to use for metrics reporting: %s, prometheus, none",
			defaultMetricsBackend))
	flags.String(
		metricsHTTPRoute,
		defaultMetricsRoute,
		"Defines the route of HTTP endpoint for metrics backends that support scraping")
}

// InitFromViper initializes Builder with properties retrieved from Viper.
func (b *Builder) InitFromViper(v *viper.Viper) {
	b.Backend = v.GetString(metricsBackend)
	b.HTTPRoute = v.GetString(metricsHTTPRoute)
}

// CreateMetricsFactory creates a metrics factory based on the configured type of the backend.
// If the metrics backend supports HTTP endpoint for scraping, it is stored in the builder and
// can be later added by RegisterHandler function.
func (b *Builder) CreateMetricsFactory(namespace string) (metrics.Factory, error) {
	if b.Backend == "prometheus" {
		metricsFactory := xkit.Wrap(namespace, kitprom.NewFactory("", "", nil))
		b.handler = promhttp.Handler()
		return metricsFactory, nil
	}
	if b.Backend == "expvar" {
		metricsFactory := xkit.Wrap(namespace, kitexpvar.NewFactory(10))
		b.handler = expvar.Handler()
		return metricsFactory, nil
	}
	if b.Backend == "none" || b.Backend == "" {
		return metrics.NullFactory, nil
	}
	return nil, errUnknownBackend
}

// RegisterHandler adds an endpoint to the mux if the metrics backend supports it.
func (b *Builder) RegisterHandler(mux *http.ServeMux) {
	if b.handler != nil && b.HTTPRoute != "" {
		mux.Handle(b.HTTPRoute, b.handler)
	}
}
