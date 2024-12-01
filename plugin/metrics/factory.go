// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"flag"
	"fmt"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/telemetry"
	"github.com/jaegertracing/jaeger/plugin"
	"github.com/jaegertracing/jaeger/plugin/metrics/disabled"
	"github.com/jaegertracing/jaeger/plugin/metrics/prometheus"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/metricsstore"
)

const (
	// disabledStorageType is the storage type used when METRICS_STORAGE_TYPE is unset.
	disabledStorageType = ""

	prometheusStorageType = "prometheus"
)

// AllStorageTypes defines all available storage backends.
var AllStorageTypes = []string{prometheusStorageType}

var _ plugin.Configurable = (*Factory)(nil)

// Factory implements storage.Factory interface as a meta-factory for storage components.
type Factory struct {
	FactoryConfig
	factories map[string]storage.MetricsFactory
}

// NewFactory creates the meta-factory.
func NewFactory(config FactoryConfig) (*Factory, error) {
	f := &Factory{FactoryConfig: config}
	uniqueTypes := map[string]struct{}{
		f.MetricsStorageType: {},
	}
	f.factories = make(map[string]storage.MetricsFactory)
	for t := range uniqueTypes {
		ff, err := f.getFactoryOfType(t)
		if err != nil {
			return nil, err
		}
		f.factories[t] = ff
	}
	return f, nil
}

func (*Factory) getFactoryOfType(factoryType string) (storage.MetricsFactory, error) {
	switch factoryType {
	case prometheusStorageType:
		return prometheus.NewFactory(), nil
	case disabledStorageType:
		return disabled.NewFactory(), nil
	}
	return nil, fmt.Errorf("unknown metrics type %q. Valid types are %v", factoryType, AllStorageTypes)
}

// Initialize implements storage.MetricsFactory.
func (f *Factory) Initialize(telset telemetry.Settings) error {
	for kind, factory := range f.factories {
		scopedTelset := telset
		scopedTelset.Metrics = telset.Metrics.Namespace(metrics.NSOptions{
			Name: "storage",
			Tags: map[string]string{
				"kind": kind,
			},
		})
		factory.Initialize(scopedTelset)
	}
	return nil
}

// CreateMetricsReader implements storage.MetricsFactory.
func (f *Factory) CreateMetricsReader() (metricsstore.Reader, error) {
	factory, ok := f.factories[f.MetricsStorageType]
	if !ok {
		return nil, fmt.Errorf("no %q backend registered for metrics store", f.MetricsStorageType)
	}
	return factory.CreateMetricsReader()
}

// AddFlags implements plugin.Configurable.
func (f *Factory) AddFlags(flagSet *flag.FlagSet) {
	for _, factory := range f.factories {
		if conf, ok := factory.(plugin.Configurable); ok {
			conf.AddFlags(flagSet)
		}
	}
}

// InitFromViper implements plugin.Configurable.
func (f *Factory) InitFromViper(v *viper.Viper, logger *zap.Logger) {
	for _, factory := range f.factories {
		if conf, ok := factory.(plugin.Configurable); ok {
			conf.InitFromViper(v, logger)
		}
	}
}
