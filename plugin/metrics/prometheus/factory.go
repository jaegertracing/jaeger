// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package prometheus

import (
	"flag"

	"github.com/spf13/viper"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/prometheus/config"
	"github.com/jaegertracing/jaeger/plugin"
	prometheusstore "github.com/jaegertracing/jaeger/plugin/metrics/prometheus/metricsstore"
	"github.com/jaegertracing/jaeger/storage/metricsstore"
	"github.com/jaegertracing/jaeger/storage/metricsstore/metricstoremetrics"
)

var _ plugin.Configurable = (*Factory)(nil)

// Factory implements storage.Factory and creates storage components backed by memory store.
type Factory struct {
	options        *Options
	logger         *zap.Logger
	tracer         trace.TracerProvider
	metricsFactory metrics.Factory
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
func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	f.metricsFactory, f.logger = metricsFactory, logger
	return nil
}

// CreateMetricsReader implements storage.MetricsFactory.
func (f *Factory) CreateMetricsReader() (metricsstore.Reader, error) {
	mr, err := prometheusstore.NewMetricsReader(f.options.Configuration, f.logger, f.tracer)
	if err != nil {
		return mr, err
	}
	return metricstoremetrics.NewReaderDecorator(mr, f.metricsFactory), nil
}

func NewFactoryWithConfig(
	cfg config.Configuration,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
) (*Factory, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	f := NewFactory()
	f.options = &Options{
		Configuration: cfg,
	}
	f.Initialize(metricsFactory, logger)
	return f, nil
}
