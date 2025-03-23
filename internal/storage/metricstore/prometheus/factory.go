// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package prometheus

import (
	"flag"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	config "github.com/jaegertracing/jaeger/internal/config/promcfg"
	prometheusstore "github.com/jaegertracing/jaeger/internal/storage/metricstore/prometheus/metricstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore/metricstoremetrics"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

var _ storage.Configurable = (*Factory)(nil)

// Factory implements storage.Factory and creates storage components backed by memory store.
type Factory struct {
	options *Options
	telset  telemetry.Settings
}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	telset := telemetry.NoopSettings()
	return &Factory{
		telset:  telset,
		options: NewOptions(),
	}
}

// AddFlags implements storage.Configurable.
func (f *Factory) AddFlags(flagSet *flag.FlagSet) {
	f.options.AddFlags(flagSet)
}

// InitFromViper implements storage.Configurable.
func (f *Factory) InitFromViper(v *viper.Viper, logger *zap.Logger) {
	if err := f.options.InitFromViper(v); err != nil {
		logger.Panic("Failed to initialize metrics storage factory", zap.Error(err))
	}
}

// Initialize implements storage.MetricsFactory.
func (f *Factory) Initialize(telset telemetry.Settings) error {
	f.telset = telset
	return nil
}

// CreateMetricsReader implements storage.MetricsFactory.
func (f *Factory) CreateMetricsReader() (metricstore.Reader, error) {
	mr, err := prometheusstore.NewMetricsReader(f.options.Configuration, f.telset.Logger, f.telset.TracerProvider)
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
	f.options = &Options{
		Configuration: cfg,
	}
	f.Initialize(telset)
	return f, nil
}
