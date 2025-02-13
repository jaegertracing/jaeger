// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package factory

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/safeexpvar"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/dependencystore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/badger"
	"github.com/jaegertracing/jaeger/internal/storage/v1/blackhole"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra"
	es "github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/v1/grpc"
	"github.com/jaegertracing/jaeger/internal/storage/v1/kafka"
	"github.com/jaegertracing/jaeger/internal/storage/v1/memory"
	"github.com/jaegertracing/jaeger/pkg/metrics"
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
	_ storage.Factory      = (*Factory)(nil)
	_ io.Closer            = (*Factory)(nil)
	_ storage.Configurable = (*Factory)(nil)
)

// Factory implements storage.Factory interface as a meta-factory for storage components.
type Factory struct {
	Config
	metricsFactory         metrics.Factory
	factories              map[string]storage.Factory
	archiveFactories       map[string]storage.Factory
	downsamplingFlagsAdded bool
}

// NewFactory creates the meta-factory.
func NewFactory(config Config) (*Factory, error) {
	f := &Factory{Config: config}
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

		if af, ok := f.getArchiveFactoryOfType(t); ok {
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

func (*Factory) getArchiveFactoryOfType(factoryType string) (storage.Factory, bool) {
	switch factoryType {
	case cassandraStorageType:
		return cassandra.NewArchiveFactory(), true
	case elasticsearchStorageType, opensearchStorageType:
		return es.NewArchiveFactory(), true
	case grpcStorageType:
		return grpc.NewArchiveFactory(), true
	default:
		return nil, false
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

	for kind, factory := range f.archiveFactories {
		if archivable, ok := factory.(storage.ArchiveCapable); ok && archivable.IsArchiveCapable() {
			if err := initializeFactory(kind, factory, "archive"); err != nil {
				return err
			}
		} else {
			delete(f.archiveFactories, kind)
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

// AddFlags implements storage.Configurable
func (f *Factory) AddFlags(flagSet *flag.FlagSet) {
	addFlags := func(factories map[string]storage.Factory) {
		for _, factory := range factories {
			if conf, ok := factory.(storage.Configurable); ok {
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

// InitFromViper implements storage.Configurable
func (f *Factory) InitFromViper(v *viper.Viper, logger *zap.Logger) {
	initializeConfigurable := func(factory storage.Factory) {
		if conf, ok := factory.(storage.Configurable); ok {
			conf.InitFromViper(v, logger)
		}
	}
	for _, factory := range f.factories {
		initializeConfigurable(factory)
	}
	for kind, factory := range f.archiveFactories {
		initializeConfigurable(factory)

		if primaryFactory, ok := f.factories[kind]; ok {
			if inheritable, ok := factory.(storage.Inheritable); ok {
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
		f.Config.DownsamplingRatio = defaultDownsamplingRatio
		f.Config.DownsamplingHashSalt = defaultDownsamplingHashSalt
		return
	}

	f.Config.DownsamplingRatio = v.GetFloat64(downsamplingRatio)
	if f.Config.DownsamplingRatio < 0 || f.Config.DownsamplingRatio > 1 {
		// Values not in the range of 0 ~ 1.0 will be set to default.
		f.Config.DownsamplingRatio = 1.0
	}
	f.Config.DownsamplingHashSalt = v.GetString(downsamplingHashSalt)
}

type ArchiveStorage struct {
	Reader spanstore.Reader
	Writer spanstore.Writer
}

func (f *Factory) InitArchiveStorage() (*ArchiveStorage, error) {
	factory, ok := f.archiveFactories[f.SpanReaderType]
	if !ok {
		return nil, nil
	}
	reader, err := factory.CreateSpanReader()
	if err != nil {
		return nil, err
	}

	factory, ok = f.archiveFactories[f.SpanWriterTypes[0]]
	if !ok {
		return nil, nil
	}
	writer, err := factory.CreateSpanWriter()
	if err != nil {
		return nil, err
	}

	return &ArchiveStorage{
		Reader: reader,
		Writer: writer,
	}, nil
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
	safeexpvar.SetInt(downsamplingRatio, int64(f.Config.DownsamplingRatio))
	safeexpvar.SetInt(spanStorageType+"-"+f.Config.SpanReaderType, 1)
}
