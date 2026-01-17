// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package prometheus

import (
	"go.opentelemetry.io/collector/extension/extensionauth"

	config "github.com/jaegertracing/jaeger/internal/config/promcfg"
	prometheusstore "github.com/jaegertracing/jaeger/internal/storage/metricstore/prometheus/metricstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore/metricstoremetrics"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

// Factory implements storage.Factory and creates storage components backed by memory store.
type Factory struct {
	options *Options
	telset  telemetry.Settings
	// httpAuth is an optional authenticator used to wrap the HTTP RoundTripper for outbound requests to Prometheus.
	httpAuth extensionauth.HTTPClient
}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	telset := telemetry.NoopSettings()
	return &Factory{
		telset:  telset,
		options: NewOptions(),
	}
}

// Initialize implements storage.V1MetricStoreFactory.
func (f *Factory) Initialize(telset telemetry.Settings) error {
	f.telset = telset
	return nil
}

// CreateMetricsReader implements storage.V1MetricStoreFactory.
func (f *Factory) CreateMetricsReader() (metricstore.Reader, error) {
	mr, err := prometheusstore.NewMetricsReader(f.options.Configuration, f.telset.Logger, f.telset.TracerProvider, f.httpAuth)
	if err != nil {
		return nil, err
	}
	return metricstoremetrics.NewReaderDecorator(mr, f.telset.Metrics), nil
}

// NewFactoryWithConfig creates a new Factory with configuration and optional HTTP authenticator.
// Pass nil for httpAuth if authentication is not required.
func NewFactoryWithConfig(
	cfg config.Configuration,
	telset telemetry.Settings,
	httpAuth extensionauth.HTTPClient,
) (*Factory, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	f := NewFactory()
	f.options = &Options{
		Configuration: cfg,
	}
	f.httpAuth = httpAuth
	_ = f.Initialize(telset)
	return f, nil
}
