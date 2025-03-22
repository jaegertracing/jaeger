// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package metricstore

import (
	"flag"
	"fmt"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/metricstore/disabled"
	"github.com/jaegertracing/jaeger/internal/storage/metricstore/prometheus"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

const (
	// disabledStorageType is the storage type used when METRICS_STORAGE_TYPE is unset.
	disabledStorageType = ""

	prometheusStorageType = "prometheus"
)

// AllStorageTypes defines all available storage backends.
var AllStorageTypes = []string{prometheusStorageType}

var _ storage.Configurable = (*Factory)(nil)

// Factory implements storage.Factory interface as a meta-factory for storage components.
type Factory struct {
	FactoryConfig
	factories map[string]storage.MetricStoreFactory
}

// NewFactory creates the meta-factory.
func NewFactory(config FactoryConfig) (*Factory, error) {
	f := &Factory{FactoryConfig: config}
	uniqueTypes := map[string]struct{}{
		f.MetricsStorageType: {},
	}
	f.factories = make(map[string]storage.MetricStoreFactory)
	for t := range uniqueTypes {
		ff, err := f.getFactoryOfType(t)
		if err != nil {
			return nil, err
		}
		f.factories[t] = ff
	}
	return f, nil
}

func (*Factory) getFactoryOfType(factoryType string) (storage.MetricStoreFactory, error) {
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
				"role": "metricstore",
			},
		})
		factory.Initialize(scopedTelset)
	}
	return nil
}

// CreateMetricsReader implements storage.MetricsFactory.
func (f *Factory) CreateMetricsReader() (metricstore.Reader, error) {
	factory, ok := f.factories[f.MetricsStorageType]
	if !ok {
		return nil, fmt.Errorf("no %q backend registered for metrics store", f.MetricsStorageType)
	}
	return factory.CreateMetricsReader()
}

// AddFlags implements storage.Configurable.
func (f *Factory) AddFlags(flagSet *flag.FlagSet) {
	for _, factory := range f.factories {
		if conf, ok := factory.(storage.Configurable); ok {
			conf.AddFlags(flagSet)
		}
	}
}

// InitFromViper implements storage.Configurable.
func (f *Factory) InitFromViper(v *viper.Viper, logger *zap.Logger) {
	for _, factory := range f.factories {
		if conf, ok := factory.(storage.Configurable); ok {
			conf.InitFromViper(v, logger)
		}
	}
}
