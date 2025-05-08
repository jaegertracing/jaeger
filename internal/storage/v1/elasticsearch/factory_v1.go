// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"flag"
	"io"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/dependencystore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/spanstoremetrics"
	esDepStorev1 "github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/dependencystore"
	esSpanStore "github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore"
)

var ( // interface comformance checks
	_ storage.Factory        = (*FactoryV1)(nil)
	_ io.Closer              = (*FactoryV1)(nil)
	_ storage.Configurable   = (*FactoryV1)(nil)
	_ storage.Inheritable    = (*FactoryV1)(nil)
	_ storage.Purger         = (*FactoryV1)(nil)
	_ storage.ArchiveCapable = (*FactoryV1)(nil)
)

type FactoryV1 struct {
	Options     *Options
	coreFactory *FactoryBase
}

func NewFactoryV1() *FactoryV1 {
	coreFactory := NewFactoryBase()
	return &FactoryV1{
		coreFactory: coreFactory,
		Options:     coreFactory.Options,
	}
}

func NewArchiveFactoryV1() *FactoryV1 {
	coreFactory := NewArchiveFactoryBase()
	return &FactoryV1{
		coreFactory: coreFactory,
		Options:     coreFactory.Options,
	}
}

func NewFactoryV1WithConfig(
	cfg config.Configuration,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
) (*FactoryV1, error) {
	coreFactory, err := NewFactoryBaseWithConfig(cfg, metricsFactory, logger)
	if err != nil {
		return nil, err
	}
	return &FactoryV1{
		coreFactory: coreFactory,
		Options:     coreFactory.Options,
	}, nil
}

func (f *FactoryV1) AddFlags(flagSet *flag.FlagSet) {
	f.Options.AddFlags(flagSet)
}

func (f *FactoryV1) InitFromViper(v *viper.Viper, _ *zap.Logger) {
	f.Options.InitFromViper(v)
	f.configureFromOptions(f.Options)
}

// configureFromOptions configures factory from Options struct.
func (f *FactoryV1) configureFromOptions(o *Options) {
	f.Options = o
	f.coreFactory.SetConfig(f.Options.GetConfig())
}

func (f *FactoryV1) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	return f.coreFactory.Initialize(metricsFactory, logger)
}

// CreateSpanReader implements storage.Factory
func (f *FactoryV1) CreateSpanReader() (spanstore.Reader, error) {
	params, err := f.coreFactory.GetSpanReaderParams()
	if err != nil {
		return nil, err
	}
	sr := esSpanStore.NewSpanReaderV1(params)
	return spanstoremetrics.NewReaderDecorator(sr, f.coreFactory.GetMetricsFactory()), nil
}

// CreateSpanWriter implements storage.Factory
func (f *FactoryV1) CreateSpanWriter() (spanstore.Writer, error) {
	params, err := f.coreFactory.GetSpanWriterParams()
	if err != nil {
		return nil, err
	}
	wr := esSpanStore.NewSpanWriterV1(params)
	err = createTemplates(wr, f.coreFactory.GetConfig())
	if err != nil {
		return nil, err
	}
	return wr, nil
}

func (f *FactoryV1) CreateDependencyReader() (dependencystore.Reader, error) {
	params := f.coreFactory.GetDependencyStoreParams()
	return esDepStorev1.NewDependencyStoreV1(params), nil
}

func (f *FactoryV1) CreateSamplingStore(maxBuckets int) (samplingstore.Store, error) {
	return f.coreFactory.CreateSamplingStore(maxBuckets)
}

func (f *FactoryV1) Close() error {
	return f.coreFactory.Close()
}

func (f *FactoryV1) Purge(ctx context.Context) error {
	return f.coreFactory.Purge(ctx)
}

func (f *FactoryV1) InheritSettingsFrom(other storage.Factory) {
	if otherFactory, ok := other.(*FactoryV1); ok {
		f.coreFactory.GetConfig().ApplyDefaults(otherFactory.coreFactory.GetConfig())
	}
}

func (f *FactoryV1) IsArchiveCapable() bool {
	return f.coreFactory.IsArchiveCapable()
}

func createTemplates(writer *esSpanStore.SpanWriterV1, cfg *config.Configuration) error {
	// Creating a template here would conflict with the one created for ILM resulting to no index rollover
	if cfg.CreateIndexTemplates && !cfg.UseILM {
		mappingBuilder := mappingBuilderFromConfig(cfg)
		spanMapping, serviceMapping, err := mappingBuilder.GetSpanServiceMappings()
		if err != nil {
			return err
		}
		if err := writer.CreateTemplates(spanMapping, serviceMapping, cfg.Indices.IndexPrefix); err != nil {
			return err
		}
	}
	return nil
}
