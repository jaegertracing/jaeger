// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/safeexpvar"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/plugin"
	"github.com/jaegertracing/jaeger/plugin/storage/badger"
	"github.com/jaegertracing/jaeger/plugin/storage/blackhole"
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
	opensearchStorageType    = "opensearch"
	elasticsearchStorageType = "elasticsearch"
	memoryStorageType        = "memory"
	kafkaStorageType         = "kafka"
	grpcStorageType          = "grpc"
	badgerStorageType        = "badger"
	blackholeStorageType     = "blackhole"

	downsamplingRatio    = "downsampling.ratio"
	downsamplingHashSalt = "downsampling.hashsalt"
	spanStorageType      = "span-storage-type"

	// defaultDownsamplingRatio is the default downsampling ratio.
	defaultDownsamplingRatio = 1.0
	// defaultDownsamplingHashSalt is the default downsampling hashsalt.
	defaultDownsamplingHashSalt = ""
)

// AllStorageTypes defines all available storage backends
var AllStorageTypes = []string{
	cassandraStorageType,
	opensearchStorageType,
	elasticsearchStorageType,
	memoryStorageType,
	kafkaStorageType,
	badgerStorageType,
	blackholeStorageType,
	grpcStorageType,
}

// AllSamplingStorageTypes returns all storage backends that implement adaptive sampling
func AllSamplingStorageTypes() []string {
	f := &Factory{}
	var backends []string
	for _, st := range AllStorageTypes {
		f, _ := f.getFactoryOfType(st) // no errors since we're looping through supported types
		if _, ok := f.(storage.SamplingStoreFactory); ok {
			backends = append(backends, st)
		}
	}
	return backends
}

var ( // interface comformance checks
	_ storage.Factory     = (*Factory)(nil)
	_ io.Closer           = (*Factory)(nil)
	_ plugin.Configurable = (*Factory)(nil)
)

// Factory implements storage.Factory interface as a meta-factory for storage components.
type Factory struct {
	FactoryConfig
	metricsFactory         metrics.Factory
	factories              map[string]storage.Factory
	archiveFactories       map[string]storage.Factory
	downsamplingFlagsAdded bool
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
	// skip SamplingStorageType if it is empty. See CreateSamplingStoreFactory for details
	if f.SamplingStorageType != "" {
		uniqueTypes[f.SamplingStorageType] = struct{}{}
	}
	f.factories = make(map[string]storage.Factory)
	f.archiveFactories = make(map[string]storage.Factory)
	for t := range uniqueTypes {
		ff, err := f.getFactoryOfType(t)
		if err != nil {
			return nil, err
		}
		f.factories[t] = ff

		af, err := f.getArchiveFactoryOfType(t)
		if err == nil {
			f.archiveFactories[t] = af
		}
	}
	return f, nil
}

func (*Factory) getFactoryOfType(factoryType string) (storage.Factory, error) {
	switch factoryType {
	case cassandraStorageType:
		return cassandra.NewFactory(), nil
	case elasticsearchStorageType, opensearchStorageType:
		return es.NewFactory(), nil
	case memoryStorageType:
		return memory.NewFactory(), nil
	case kafkaStorageType:
		return kafka.NewFactory(), nil
	case badgerStorageType:
		return badger.NewFactory(), nil
	case grpcStorageType:
		return grpc.NewFactory(), nil
	case blackholeStorageType:
		return blackhole.NewFactory(), nil
	default:
		return nil, fmt.Errorf("unknown storage type %s. Valid types are %v", factoryType, AllStorageTypes)
	}
}

func (*Factory) getArchiveFactoryOfType(factoryType string) (storage.Factory, error) {
	switch factoryType {
	case cassandraStorageType:
		return cassandra.NewArchiveFactory(), nil
	case elasticsearchStorageType, opensearchStorageType:
		return es.NewArchiveFactory(), nil
	case grpcStorageType:
		return grpc.NewArchiveFactory(), nil
	default:
		return nil, fmt.Errorf("archive storage factory not available for %s", factoryType)
	}
}

func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	f.metricsFactory = metricsFactory

	initializeFactory := func(kind string, factory storage.Factory, role string) error {
		mf := metricsFactory.Namespace(metrics.NSOptions{
			Name: "storage",
			Tags: map[string]string{
				"kind": kind,
				"role": role,
			},
		})
		return factory.Initialize(mf, logger)
	}

	for kind, factory := range f.factories {
		if err := initializeFactory(kind, factory, "primary"); err != nil {
			return err
		}
	}

	uninitializedArchiveFactories := make(map[string]struct{})
	for kind, factory := range f.archiveFactories {
		if archivable, ok := factory.(plugin.ArchiveCapable); ok && archivable.IsArchiveCapable() {
			if err := initializeFactory(kind, factory, "primary"); err != nil {
				uninitializedArchiveFactories[kind] = struct{}{}
			}
		}
	}

	for kind := range uninitializedArchiveFactories {
		delete(f.archiveFactories, kind)
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

// CreateSamplingStoreFactory creates a distributedlock.Lock and samplingstore.Store for use with adaptive sampling
func (f *Factory) CreateSamplingStoreFactory() (storage.SamplingStoreFactory, error) {
	// if a sampling storage type was specified then use it, otherwise search all factories
	// for compatibility
	if f.SamplingStorageType != "" {
		factory, ok := f.factories[f.SamplingStorageType]
		if !ok {
			return nil, fmt.Errorf("no %s backend registered for sampling store", f.SamplingStorageType)
		}
		ss, ok := factory.(storage.SamplingStoreFactory)
		if !ok {
			return nil, fmt.Errorf("storage factory of type %s does not support sampling store", f.SamplingStorageType)
		}
		return ss, nil
	}

	for _, factory := range f.factories {
		ss, ok := factory.(storage.SamplingStoreFactory)
		if ok {
			return ss, nil
		}
	}

	// returning nothing is valid here. it's quite possible that the user has no backend that can support adaptive sampling
	// this is fine as long as adaptive sampling is also not configured
	return nil, nil
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
	addFlags := func(factories map[string]storage.Factory) {
		for _, factory := range factories {
			if conf, ok := factory.(plugin.Configurable); ok {
				conf.AddFlags(flagSet)
			}
		}
	}
	addFlags(f.factories)
	addFlags(f.archiveFactories)
}

// AddPipelineFlags adds all the standard flags as well as the downsampling
// flags. This is intended to be used in Jaeger pipeline services such as
// the collector or ingester.
func (f *Factory) AddPipelineFlags(flagSet *flag.FlagSet) {
	f.AddFlags(flagSet)
	f.addDownsamplingFlags(flagSet)
}

// addDownsamplingFlags add flags for Downsampling params
func (f *Factory) addDownsamplingFlags(flagSet *flag.FlagSet) {
	f.downsamplingFlagsAdded = true
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
func (f *Factory) InitFromViper(v *viper.Viper, logger *zap.Logger) {
	initializeConfigurable := func(factory storage.Factory) {
		if conf, ok := factory.(plugin.Configurable); ok {
			conf.InitFromViper(v, logger)
		}
	}
	for _, factory := range f.factories {
		initializeConfigurable(factory)
	}
	for kind, factory := range f.archiveFactories {
		initializeConfigurable(factory)

		if primaryFactory, ok := f.factories[kind]; ok {
			if inheritable, ok := factory.(plugin.Inheritable); ok {
				inheritable.InheritSettingsFrom(primaryFactory)
			}
		}
	}
	f.initDownsamplingFromViper(v)
}

func (f *Factory) initDownsamplingFromViper(v *viper.Viper) {
	// if the downsampling flag isn't set then this component used the standard "AddFlags" method
	// and has no use for downsampling.  the default settings effectively disable downsampling
	if !f.downsamplingFlagsAdded {
		f.FactoryConfig.DownsamplingRatio = defaultDownsamplingRatio
		f.FactoryConfig.DownsamplingHashSalt = defaultDownsamplingHashSalt
		return
	}

	f.FactoryConfig.DownsamplingRatio = v.GetFloat64(downsamplingRatio)
	if f.FactoryConfig.DownsamplingRatio < 0 || f.FactoryConfig.DownsamplingRatio > 1 {
		// Values not in the range of 0 ~ 1.0 will be set to default.
		f.FactoryConfig.DownsamplingRatio = 1.0
	}
	f.FactoryConfig.DownsamplingHashSalt = v.GetString(downsamplingHashSalt)
}

func (f *Factory) createArchiveSpanReader() (spanstore.Reader, error) {
	factory, ok := f.archiveFactories[f.SpanReaderType]
	if !ok {
		return nil, fmt.Errorf("no %s backend registered for span store", f.SpanReaderType)
	}
	return factory.CreateSpanReader()
}

func (f *Factory) createArchiveSpanWriter() (spanstore.Writer, error) {
	factory, ok := f.archiveFactories[f.SpanWriterTypes[0]]
	if !ok {
		return nil, fmt.Errorf("no %s backend registered for span store", f.SpanWriterTypes[0])
	}
	return factory.CreateSpanWriter()
}

func (f *Factory) InitArchiveStorage(
	logger *zap.Logger,
) (spanstore.Reader, spanstore.Writer) {
	reader, err := f.createArchiveSpanReader()
	if err != nil {
		logger.Error("Cannot init archive storage reader", zap.Error(err))
		return nil, nil
	}
	writer, err := f.createArchiveSpanWriter()
	if err != nil {
		logger.Error("Cannot init archive storage writer", zap.Error(err))
		return nil, nil
	}

	return reader, writer
}

var _ io.Closer = (*Factory)(nil)

// Close closes the resources held by the factory
func (f *Factory) Close() error {
	var errs []error
	closeFactory := func(factory storage.Factory) {
		if closer, ok := factory.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	for _, storageType := range f.SpanWriterTypes {
		if factory, ok := f.factories[storageType]; ok {
			closeFactory(factory)
		}
		if factory, ok := f.archiveFactories[storageType]; ok {
			closeFactory(factory)
		}
	}
	return errors.Join(errs...)
}

func (f *Factory) publishOpts() {
	safeexpvar.SetInt(downsamplingRatio, int64(f.FactoryConfig.DownsamplingRatio))
	safeexpvar.SetInt(spanStorageType+"-"+f.FactoryConfig.SpanReaderType, 1)
}
