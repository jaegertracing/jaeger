// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package metrics

import (
	"errors"
	"flag"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	xkit "github.com/uber/jaeger-lib/metrics/go-kit"
	kitexpvar "github.com/uber/jaeger-lib/metrics/go-kit/expvar"
	kitprom "github.com/uber/jaeger-lib/metrics/go-kit/prometheus"
	"github.com/uber/jaeger/examples/hotrod/pkg/httpexpvar"

	"github.com/uber/jaeger-lib/metrics"
)

var errUnknownBackend = errors.New("unknown metrics backend specified")

// Builder provides command line options to configure metrics backend used by Jaeger executables.
type Builder struct {
	Backend   string
	HTTPRoute string

	handler http.Handler
}

// Bind defines command line flags and binds their values to the builder fields.
func (b *Builder) Bind(flagSet *flag.FlagSet) {
	flagSet.StringVar(
		&b.Backend,
		"metrics-backend",
		"expvar",
		"Defines which metrics backend to use for metrics reporting: prometheus, expvar, none")
	flagSet.StringVar(
		&b.HTTPRoute,
		"metrics-http-route",
		"/debug/vars",
		"Defines the route of HTTP endpoint for metrics backends that support scraping")
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
		// TODO register official expvar handler once we upgrade to Go 1.8
		b.handler = http.HandlerFunc(httpexpvar.Handler)
		return metricsFactory, nil
	}
	if b.Backend == "none" {
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
