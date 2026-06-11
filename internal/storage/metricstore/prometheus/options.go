// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package prometheus

import (
	"time"

	config "github.com/jaegertracing/jaeger/internal/config/promcfg"
)

const (
	defaultServerURL      = "http://localhost:9090"
	defaultConnectTimeout = 30 * time.Second

	// the default configuration here matches the default namespace in the span metrics connector
	defaultMetricNamespace   = "traces_span_metrics"
	defaultLatencyUnit       = "ms"
	defaultNormalizeCalls    = false
	defaultNormalizeDuration = false
)

// Options is an alias for the Prometheus storage configuration.
type Options = config.Configuration

func DefaultConfig() config.Configuration {
	return config.Configuration{
		ServerURL:      defaultServerURL,
		ConnectTimeout: defaultConnectTimeout,

		MetricNamespace:   defaultMetricNamespace,
		LatencyUnit:       defaultLatencyUnit,
		NormalizeCalls:    defaultNormalizeCalls,
		NormalizeDuration: defaultNormalizeDuration,
	}
}

// NewOptions creates a new Options with default values.
func NewOptions() *Options {
	cfg := DefaultConfig()
	return &cfg
}
