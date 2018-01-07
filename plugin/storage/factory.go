// Copyright (c) 2017 Uber Technologies, Inc.
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

package storage

import (
	"flag"
	"fmt"
	"os"

	"github.com/spf13/viper"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/plugin"
	"github.com/jaegertracing/jaeger/plugin/storage/cassandra"
	"github.com/jaegertracing/jaeger/plugin/storage/es"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	// SpanStorageEnvVar is the name of the env var that defines the type of backend used for span storage.
	SpanStorageEnvVar       = "SPAN_STORAGE"
	dependencyStorageEnvVar = "DEPENDENCY_STORAGE"

	cassandraStorageType     = "cassandra"
	elasticsearchStorageType = "elasticsearch"
	memoryStorageType        = "memory"
)

var allStorageTypes = []string{cassandraStorageType, elasticsearchStorageType, memoryStorageType}

// Factory implements storage.Factory interface as a meta-factory for storage components.
// It reads the desired types of storage backends from SPAN_STORAGE and DEPENDENCY_STORAGE
// environment variable. Allowed values:
//   * `cassandra` - built-in
//   * `elasticsearch` - built-in
//   * `memory` - built-in
//   * `plugin` - loads a dynamic plugin that implements storage.Factory interface (not supported at the moment)
type Factory struct {
	spanStoreType string
	depStoreType  string

	factories map[string]storage.Factory
}

// NewFactory creates the meta-factory.
func NewFactory() (*Factory, error) {
	f := &Factory{}
	f.spanStoreType = os.Getenv(SpanStorageEnvVar)
	if f.spanStoreType == "" {
		f.spanStoreType = cassandraStorageType
	}
	f.depStoreType = os.Getenv(dependencyStorageEnvVar)
	if f.depStoreType == "" {
		f.depStoreType = f.spanStoreType
	}
	uniqueTypes := map[string]struct{}{
		f.spanStoreType: {},
		f.depStoreType:  {},
	}
	f.factories = make(map[string]storage.Factory)
	for t := range uniqueTypes {
		ff, err := f.getFactoryOfType(t)
		if err != nil {
			return nil, err
		}
		f.factories[t] = ff
	}
	return f, nil
}

func (f *Factory) getFactoryOfType(factoryType string) (storage.Factory, error) {
	switch factoryType {
	case cassandraStorageType:
		return cassandra.NewFactory(), nil
	case elasticsearchStorageType:
		return es.NewFactory(), nil
	case memoryStorageType:
		return memory.NewFactory(), nil
	default:
		return nil, fmt.Errorf("Unknown storage type %s. Valid types are %v", factoryType, allStorageTypes)
	}
}

// Initialize implements storage.Factory
func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	for _, factory := range f.factories {
		if err := factory.Initialize(metricsFactory, logger); err != nil {
			return err
		}
	}
	return nil
}

// CreateSpanReader implements storage.Factory
func (f *Factory) CreateSpanReader() (spanstore.Reader, error) {
	factory, ok := f.factories[f.spanStoreType]
	if !ok {
		return nil, fmt.Errorf("No %s backend registered for span store", f.spanStoreType)
	}
	return factory.CreateSpanReader()
}

// CreateSpanWriter implements storage.Factory
func (f *Factory) CreateSpanWriter() (spanstore.Writer, error) {
	factory, ok := f.factories[f.spanStoreType]
	if !ok {
		return nil, fmt.Errorf("No %s backend registered for span store", f.spanStoreType)
	}
	return factory.CreateSpanWriter()
}

// CreateDependencyReader implements storage.Factory
func (f *Factory) CreateDependencyReader() (dependencystore.Reader, error) {
	factory, ok := f.factories[f.spanStoreType]
	if !ok {
		return nil, fmt.Errorf("No %s backend registered for span store", f.spanStoreType)
	}
	return factory.CreateDependencyReader()
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
func (f *Factory) InitFromViper(v *viper.Viper) {
	for _, factory := range f.factories {
		if conf, ok := factory.(plugin.Configurable); ok {
			conf.InitFromViper(v)
		}
	}
}
