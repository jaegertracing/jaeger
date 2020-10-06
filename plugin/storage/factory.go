// Copyright (c) 2019 The Jaeger Authors.
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
	"io"

	"github.com/spf13/viper"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/multierror"
	"github.com/jaegertracing/jaeger/plugin"
	"github.com/jaegertracing/jaeger/plugin/storage/badger"
	"github.com/jaegertracing/jaeger/plugin/storage/cassandra"
	"github.com/jaegertracing/jaeger/plugin/storage/es"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc"
	"github.com/jaegertracing/jaeger/plugin/storage/kafka"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	cassandraStorageType     = "cassandra"
	elasticsearchStorageType = "elasticsearch"
	memoryStorageType        = "memory"
	kafkaStorageType         = "kafka"
	grpcPluginStorageType    = "grpc-plugin"
	badgerStorageType        = "badger"
	downsamplingRatio        = "downsampling.ratio"
	downsamplingHashSalt     = "downsampling.hashsalt"
	spanStorageType          = "span-storage-type"

	// defaultDownsamplingRatio is the default downsampling ratio.
	defaultDownsamplingRatio = 1.0
	// defaultDownsamplingHashSalt is the default downsampling hashsalt.
	defaultDownsamplingHashSalt = ""
)

// AllStorageTypes defines all available storage backends
var AllStorageTypes = []string{cassandraStorageType, elasticsearchStorageType, memoryStorageType, kafkaStorageType, badgerStorageType, grpcPluginStorageType}

// Factory implements storage.Factory interface as a meta-factory for storage components.
type Factory struct {
	FactoryConfig
	metricsFactory metrics.Factory
	factories      map[string]storage.Factory
}

// NewFactory creates the meta-factory.
func NewFactory(config FactoryConfig) (*Factory, error) {
	f := &Factory{FactoryConfig: config}
	uniqueTypes := map[string]struct{}{
		f.SpanReaderType:          {},
		f.DependenciesStorageType: {},
	}
	for _, storageType := range f.SpanWriterTypes {
		uniqueTypes[storageType] = struct{}{}
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
	case kafkaStorageType:
		return kafka.NewFactory(), nil
	case badgerStorageType:
		return badger.NewFactory(), nil
	case grpcPluginStorageType:
		return grpc.NewFactory(), nil
	default:
		return nil, fmt.Errorf("unknown storage type %s. Valid types are %v", factoryType, AllStorageTypes)
	}
}

// Initialize implements storage.Factory.
func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	f.metricsFactory = metricsFactory
	for _, factory := range f.factories {
		if err := factory.Initialize(metricsFactory, logger); err != nil {
			return err
		}
	}
	f.publishOpts()

	return nil
}

// CreateSpanReader implements storage.Factory.
func (f *Factory) CreateSpanReader() (spanstore.Reader, error) {
	factory, ok := f.factories[f.SpanReaderType]
	if !ok {
		return nil, fmt.Errorf("no %s backend registered for span store", f.SpanReaderType)
	}
	return factory.CreateSpanReader()
}

// CreateSpanWriter implements storage.Factory.
func (f *Factory) CreateSpanWriter() (spanstore.Writer, error) {
	var writers []spanstore.Writer
	for _, storageType := range f.SpanWriterTypes {
		factory, ok := f.factories[storageType]
		if !ok {
			return nil, fmt.Errorf("no %s backend registered for span store", storageType)
		}
		writer, err := factory.CreateSpanWriter()
		if err != nil {
			return nil, err
		}
		writers = append(writers, writer)
	}
	var spanWriter spanstore.Writer
	if len(f.SpanWriterTypes) == 1 {
		spanWriter = writers[0]
	} else {
		spanWriter = spanstore.NewCompositeWriter(writers...)
	}
	// Turn off DownsamplingWriter entirely if ratio == defaultDownsamplingRatio.
	if f.DownsamplingRatio == defaultDownsamplingRatio {
		return spanWriter, nil
	}
	return spanstore.NewDownsamplingWriter(spanWriter, spanstore.DownsamplingOptions{
		Ratio:          f.DownsamplingRatio,
		HashSalt:       f.DownsamplingHashSalt,
		MetricsFactory: f.metricsFactory.Namespace(metrics.NSOptions{Name: "downsampling_writer"}),
	}), nil
}

// CreateDependencyReader implements storage.Factory
func (f *Factory) CreateDependencyReader() (dependencystore.Reader, error) {
	factory, ok := f.factories[f.DependenciesStorageType]
	if !ok {
		return nil, fmt.Errorf("no %s backend registered for span store", f.DependenciesStorageType)
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
	addDownsamplingFlags(flagSet)
}

// addDownsamplingFlags add flags for Downsampling params
func addDownsamplingFlags(flagSet *flag.FlagSet) {
	flagSet.Float64(
		downsamplingRatio,
		defaultDownsamplingRatio,
		"Ratio of spans passed to storage after downsampling (between 0 and 1), e.g ratio = 0.3 means we are keeping 30% of spans and dropping 70% of spans; ratio = 1.0 disables downsampling.",
	)
	flagSet.String(
		downsamplingHashSalt,
		defaultDownsamplingHashSalt,
		"Salt used when hashing trace id for downsampling.",
	)
}

// InitFromViper implements plugin.Configurable
func (f *Factory) InitFromViper(v *viper.Viper) {
	for _, factory := range f.factories {
		if conf, ok := factory.(plugin.Configurable); ok {
			conf.InitFromViper(v)
		}
	}
	f.initDownsamplingFromViper(v)
}

func (f *Factory) initDownsamplingFromViper(v *viper.Viper) {
	f.FactoryConfig.DownsamplingRatio = v.GetFloat64(downsamplingRatio)
	if f.FactoryConfig.DownsamplingRatio < 0 || f.FactoryConfig.DownsamplingRatio > 1 {
		// Values not in the range of 0 ~ 1.0 will be set to default.
		f.FactoryConfig.DownsamplingRatio = 1.0
	}
	f.FactoryConfig.DownsamplingHashSalt = v.GetString(downsamplingHashSalt)
}

// CreateArchiveSpanReader implements storage.ArchiveFactory
func (f *Factory) CreateArchiveSpanReader() (spanstore.Reader, error) {
	factory, ok := f.factories[f.SpanReaderType]
	if !ok {
		return nil, fmt.Errorf("no %s backend registered for span store", f.SpanReaderType)
	}
	archive, ok := factory.(storage.ArchiveFactory)
	if !ok {
		return nil, storage.ErrArchiveStorageNotSupported
	}
	return archive.CreateArchiveSpanReader()
}

// CreateArchiveSpanWriter implements storage.ArchiveFactory
func (f *Factory) CreateArchiveSpanWriter() (spanstore.Writer, error) {
	factory, ok := f.factories[f.SpanWriterTypes[0]]
	if !ok {
		return nil, fmt.Errorf("no %s backend registered for span store", f.SpanWriterTypes[0])
	}
	archive, ok := factory.(storage.ArchiveFactory)
	if !ok {
		return nil, storage.ErrArchiveStorageNotSupported
	}
	return archive.CreateArchiveSpanWriter()
}

var _ io.Closer = (*Factory)(nil)

// Close closes the resources held by the factory
func (f *Factory) Close() error {
	var errs []error
	for _, storageType := range f.SpanWriterTypes {
		if factory, ok := f.factories[storageType]; ok {
			if closer, ok := factory.(io.Closer); ok {
				err := closer.Close()
				if err != nil {
					errs = append(errs, err)
				}
			}
		}
	}
	return multierror.Wrap(errs)
}

func (f *Factory) publishOpts() {
	internalFactory := f.metricsFactory.Namespace(metrics.NSOptions{Name: "internal"})
	internalFactory.Gauge(metrics.Options{Name: downsamplingRatio}).
		Update(int64(f.FactoryConfig.DownsamplingRatio))
	internalFactory.Gauge(metrics.Options{Name: spanStorageType + "-" + f.SpanReaderType}).
		Update(1)
}
