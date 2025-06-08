// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"flag"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore/metricstoremetrics"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

var _ storage.Configurable = (*Factory)(nil)

// Factory implements storage.Factory and creates storage components backed by memory store.
type Factory struct {
	config config.Configuration
	telset telemetry.Settings
}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	telset := telemetry.NoopSettings()
	return &Factory{
		telset: telset,
	}
}

// AddFlags implements storage.Configurable.
func (*Factory) AddFlags(_ *flag.FlagSet) {}

// InitFromViper implements storage.Configurable.
func (*Factory) InitFromViper(_ *viper.Viper, _ *zap.Logger) {}

// Initialize implements storage.MetricsFactory.
func (f *Factory) Initialize(telset telemetry.Settings) error {
	f.telset = telset
	return nil
}

// CreateMetricsReader implements storage.MetricsFactory.
func (f *Factory) CreateMetricsReader() (metricstore.Reader, error) {
	mr, err := NewMetricsReader(f.config, f.telset.Logger, f.telset.TracerProvider)
	if err != nil {
		return nil, err
	}
	return metricstoremetrics.NewReaderDecorator(mr, f.telset.Metrics), nil
}

func NewFactoryWithConfig(
	cfg config.Configuration,
	telset telemetry.Settings,
) (*Factory, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	f := NewFactory()
	f.config = cfg
	f.Initialize(telset)
	return f, nil
}
