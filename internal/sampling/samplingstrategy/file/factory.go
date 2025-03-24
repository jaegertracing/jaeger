// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package file

import (
	"flag"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/sampling/samplingstrategy"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
)

var _ storage.Configurable = (*Factory)(nil)

// Factory implements samplingstrategy.Factory for a static strategy store.
type Factory struct {
	options *Options
	logger  *zap.Logger
}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	return &Factory{
		options: &Options{},
		logger:  zap.NewNop(),
	}
}

// AddFlags implements storage.Configurable
func (*Factory) AddFlags(flagSet *flag.FlagSet) {
	AddFlags(flagSet)
}

// InitFromViper implements storage.Configurable
func (f *Factory) InitFromViper(v *viper.Viper, _ *zap.Logger) {
	f.options.InitFromViper(v)
}

// Initialize implements samplingstrategy.Factory
func (f *Factory) Initialize(_ metrics.Factory, _ storage.SamplingStoreFactory, logger *zap.Logger) error {
	f.logger = logger
	return nil
}

// CreateStrategyProvider implements samplingstrategy.Factory
func (f *Factory) CreateStrategyProvider() (samplingstrategy.Provider, samplingstrategy.Aggregator, error) {
	s, err := NewProvider(*f.options, f.logger)
	if err != nil {
		return nil, nil, err
	}

	return s, nil, nil
}

// Close closes the factory.
func (*Factory) Close() error {
	return nil
}
