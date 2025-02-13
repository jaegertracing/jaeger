// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package metafactory

import (
	"errors"
	"flag"
	"fmt"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/sampling/samplingstrategy"
	"github.com/jaegertracing/jaeger/internal/sampling/samplingstrategy/adaptive"
	"github.com/jaegertracing/jaeger/internal/sampling/samplingstrategy/file"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

// Kind is a datatype holding the type of strategy store.
type Kind string

const (
	samplingTypeAdaptive = "adaptive"
	samplingTypeFile     = "file"
)

// AllSamplingTypes lists all types of sampling factories.
var AllSamplingTypes = []string{samplingTypeFile, samplingTypeAdaptive}

var (
	_ storage.Configurable     = (*Factory)(nil)
	_ samplingstrategy.Factory = (*Factory)(nil)
)

// Factory implements samplingstrategy.Factory interface as a meta-factory for strategy storage components.
type Factory struct {
	FactoryConfig

	factories map[Kind]samplingstrategy.Factory
}

// NewFactory creates the meta-factory.
func NewFactory(config FactoryConfig) (*Factory, error) {
	f := &Factory{FactoryConfig: config}
	uniqueTypes := map[Kind]struct{}{
		f.StrategyStoreType: {},
	}
	f.factories = make(map[Kind]samplingstrategy.Factory)
	for t := range uniqueTypes {
		ff, err := f.getFactoryOfType(t)
		if err != nil {
			return nil, err
		}
		f.factories[t] = ff
	}
	return f, nil
}

func (*Factory) getFactoryOfType(factoryType Kind) (samplingstrategy.Factory, error) {
	switch factoryType {
	case samplingTypeFile:
		return file.NewFactory(), nil
	case samplingTypeAdaptive:
		return adaptive.NewFactory(), nil
	default:
		return nil, fmt.Errorf("unknown sampling strategy store type %s. Valid types are %v", factoryType, AllSamplingTypes)
	}
}

// AddFlags implements storage.Configurable
func (f *Factory) AddFlags(flagSet *flag.FlagSet) {
	for _, factory := range f.factories {
		if conf, ok := factory.(storage.Configurable); ok {
			conf.AddFlags(flagSet)
		}
	}
}

// InitFromViper implements storage.Configurable
func (f *Factory) InitFromViper(v *viper.Viper, logger *zap.Logger) {
	for _, factory := range f.factories {
		if conf, ok := factory.(storage.Configurable); ok {
			conf.InitFromViper(v, logger)
		}
	}
}

// Initialize implements samplingstrategy.Factory
func (f *Factory) Initialize(metricsFactory metrics.Factory, ssFactory storage.SamplingStoreFactory, logger *zap.Logger) error {
	for _, factory := range f.factories {
		if err := factory.Initialize(metricsFactory, ssFactory, logger); err != nil {
			return err
		}
	}
	return nil
}

// CreateStrategyProvider implements samplingstrategy.Factory
func (f *Factory) CreateStrategyProvider() (samplingstrategy.Provider, samplingstrategy.Aggregator, error) {
	factory, ok := f.factories[f.StrategyStoreType]
	if !ok {
		return nil, nil, fmt.Errorf("no %s strategy store registered", f.StrategyStoreType)
	}
	return factory.CreateStrategyProvider()
}

// Close closes all factories.
func (f *Factory) Close() error {
	var errs []error
	for _, factory := range f.factories {
		if err := factory.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
