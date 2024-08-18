// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package prometheus

import (
	"flag"

	"github.com/spf13/viper"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/prometheus/config"
	"github.com/jaegertracing/jaeger/plugin"
	prometheusstore "github.com/jaegertracing/jaeger/plugin/metrics/prometheus/metricsstore"
	"github.com/jaegertracing/jaeger/storage/metricsstore"
)

var _ plugin.Configurable = (*Factory)(nil)

// Factory implements storage.Factory and creates storage components backed by memory store.
type Factory struct {
	options *Options
	logger  *zap.Logger
	tracer  trace.TracerProvider
}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	return &Factory{
		tracer:  otel.GetTracerProvider(),
		options: NewOptions(),
	}
}

// AddFlags implements plugin.Configurable.
func (f *Factory) AddFlags(flagSet *flag.FlagSet) {
	f.options.AddFlags(flagSet)
}

// InitFromViper implements plugin.Configurable.
func (f *Factory) InitFromViper(v *viper.Viper, logger *zap.Logger) {
	if err := f.options.InitFromViper(v); err != nil {
		logger.Panic("Failed to initialize metrics storage factory", zap.Error(err))
	}
}

// Initialize implements storage.MetricsFactory.
func (f *Factory) Initialize(logger *zap.Logger) error {
	f.logger = logger
	return nil
}

// CreateMetricsReader implements storage.MetricsFactory.
func (f *Factory) CreateMetricsReader() (metricsstore.Reader, error) {
	return prometheusstore.NewMetricsReader(f.options.Configuration, f.logger, f.tracer)
}

func NewFactoryWithConfig(
	cfg config.Configuration,
	logger *zap.Logger,
) (*Factory, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	f := NewFactory()
	f.options = &Options{
		Configuration: cfg,
	}
	f.Initialize(logger)
	return f, nil
}
