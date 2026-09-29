// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package metricsbuilder

import (
	"errors"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/jaegertracing/jaeger/internal/metrics"
	jprom "github.com/jaegertracing/jaeger/internal/metrics/prometheus"
)

const (
	defaultMetricsBackend = "prometheus"
	defaultMetricsRoute   = "/metrics"
)

var errUnknownBackend = errors.New("unknown metrics backend specified")

// Builder is the metrics section of a service's configuration file, and builds the
// metrics factory it describes.
type Builder struct {
	// Backend is the metrics backend to report to: "prometheus" or "none".
	Backend string `mapstructure:"backend"`
	// HTTPRoute is the path the metrics are exposed on for scraping, e.g. /metrics.
	HTTPRoute string `mapstructure:"http_route"`
	handler   http.Handler
}

// Default returns the settings a service reports metrics with unless configured otherwise:
// Prometheus on /metrics.
func Default() Builder {
	return Builder{
		Backend:   defaultMetricsBackend,
		HTTPRoute: defaultMetricsRoute,
	}
}

// CreateMetricsFactory creates a metrics factory based on the configured type of the backend.
// If the metrics backend supports HTTP endpoint for scraping, it is stored in the builder and
// can be later added by RegisterHandler function.
func (b *Builder) CreateMetricsFactory(namespace string) (metrics.Factory, error) {
	if b.Backend == "prometheus" {
		metricsFactory := jprom.New().Namespace(metrics.NSOptions{Name: namespace, Tags: nil})
		b.handler = promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{DisableCompression: true})
		return metricsFactory, nil
	}
	if b.Backend == "none" || b.Backend == "" {
		return metrics.NullFactory, nil
	}
	return nil, errUnknownBackend
}

// Handler returns an http.Handler for the metrics endpoint.
func (b *Builder) Handler() http.Handler {
	return b.handler
}
