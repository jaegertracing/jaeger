// Copyright (c) 2018 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package strategystore

import (
	"flag"
	"fmt"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/strategystore"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/plugin"
	"github.com/jaegertracing/jaeger/plugin/sampling/strategystore/adaptive"
	"github.com/jaegertracing/jaeger/plugin/sampling/strategystore/static"
	"github.com/jaegertracing/jaeger/storage"
)

// Kind is a datatype holding the type of strategy store.
type Kind string

const (
	samplingTypeAdaptive = "adaptive"
	samplingTypeFile     = "file"
)

// AllSamplingTypes lists all types of sampling factories.
var AllSamplingTypes = []string{samplingTypeFile, samplingTypeAdaptive}

var _ plugin.Configurable = (*Factory)(nil)

// Factory implements strategystore.Factory interface as a meta-factory for strategy storage components.
type Factory struct {
	FactoryConfig

	factories map[Kind]strategystore.Factory
}

// NewFactory creates the meta-factory.
func NewFactory(config FactoryConfig) (*Factory, error) {
	f := &Factory{FactoryConfig: config}
	uniqueTypes := map[Kind]struct{}{
		f.StrategyStoreType: {},
	}
	f.factories = make(map[Kind]strategystore.Factory)
	for t := range uniqueTypes {
		ff, err := f.getFactoryOfType(t)
		if err != nil {
			return nil, err
		}
		f.factories[t] = ff
	}
	return f, nil
}

func (f *Factory) getFactoryOfType(factoryType Kind) (strategystore.Factory, error) {
	switch factoryType {
	case samplingTypeFile:
		return static.NewFactory(), nil
	case samplingTypeAdaptive:
		return adaptive.NewFactory(), nil
	default:
		return nil, fmt.Errorf("unknown sampling strategy store type %s. Valid types are %v", factoryType, AllSamplingTypes)
	}
}

// AddFlags implements plugin.Configurable
func (f *Factory) AddFlags(flagSet *flag.FlagSet) {
	for _, factory := range f.factories {
		if conf, ok := factory.(plugin.Configurable); ok {
			conf.AddFlags(flagSet)
		}
	}
}

// InitFromViper implements plugin.Configurable
func (f *Factory) InitFromViper(v *viper.Viper, logger *zap.Logger) {
	for _, factory := range f.factories {
		if conf, ok := factory.(plugin.Configurable); ok {
			conf.InitFromViper(v, logger)
		}
	}
}

// Initialize implements strategystore.Factory
func (f *Factory) Initialize(metricsFactory metrics.Factory, ssFactory storage.SamplingStoreFactory, logger *zap.Logger) error {
	for _, factory := range f.factories {
		if err := factory.Initialize(metricsFactory, ssFactory, logger); err != nil {
			return err
		}
	}
	return nil
}

// CreateStrategyStore implements strategystore.Factory
func (f *Factory) CreateStrategyStore() (strategystore.StrategyStore, strategystore.Aggregator, error) {
	factory, ok := f.factories[f.StrategyStoreType]
	if !ok {
		return nil, nil, fmt.Errorf("no %s strategy store registered", f.StrategyStoreType)
	}
	return factory.CreateStrategyStore()
}
