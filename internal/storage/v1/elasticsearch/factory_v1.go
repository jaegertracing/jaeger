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
	_ storage.Factory        = (*Factory)(nil)
	_ io.Closer              = (*Factory)(nil)
	_ storage.Configurable   = (*Factory)(nil)
	_ storage.Inheritable    = (*Factory)(nil)
	_ storage.Purger         = (*Factory)(nil)
	_ storage.ArchiveCapable = (*Factory)(nil)
)

type Factory struct {
	Options     *Options
	coreFactory *FactoryBase
}

func NewFactory() *Factory {
	coreFactory := NewFactoryBase()
	return &Factory{
		coreFactory: coreFactory,
		Options:     coreFactory.Options,
	}
}

func NewArchiveFactory() *Factory {
	coreFactory := NewArchiveFactoryBase()
	return &Factory{
		coreFactory: coreFactory,
		Options:     coreFactory.Options,
	}
}

func (f *Factory) AddFlags(flagSet *flag.FlagSet) {
	f.Options.AddFlags(flagSet)
}

func (f *Factory) InitFromViper(v *viper.Viper, _ *zap.Logger) {
	f.Options.InitFromViper(v)
	f.configureFromOptions(f.Options)
}

// configureFromOptions configures factory from Options struct.
func (f *Factory) configureFromOptions(o *Options) {
	f.Options = o
	f.coreFactory.SetConfig(f.Options.GetConfig())
}

func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	return f.coreFactory.Initialize(metricsFactory, logger)
}

// CreateSpanReader implements storage.Factory
func (f *Factory) CreateSpanReader() (spanstore.Reader, error) {
	params, err := f.coreFactory.GetSpanReaderParams()
	if err != nil {
		return nil, err
	}
	sr := esSpanStore.NewSpanReaderV1(params)
	return spanstoremetrics.NewReaderDecorator(sr, f.coreFactory.GetMetricsFactory()), nil
}

// CreateSpanWriter implements storage.Factory
func (f *Factory) CreateSpanWriter() (spanstore.Writer, error) {
	params, err := f.coreFactory.GetSpanWriterParams()
	if err != nil {
		return nil, err
	}
	wr := esSpanStore.NewSpanWriterV1(params)
	err = f.createTemplates(wr, f.coreFactory.GetConfig())
	if err != nil {
		return nil, err
	}
	return wr, nil
}

func (f *Factory) CreateDependencyReader() (dependencystore.Reader, error) {
	params := f.coreFactory.GetDependencyStoreParams()
	return esDepStorev1.NewDependencyStoreV1(params), nil
}

func (f *Factory) CreateSamplingStore(maxBuckets int) (samplingstore.Store, error) {
	return f.coreFactory.CreateSamplingStore(maxBuckets)
}

func (f *Factory) Close() error {
	return f.coreFactory.Close()
}

func (f *Factory) Purge(ctx context.Context) error {
	return f.coreFactory.Purge(ctx)
}

func (f *Factory) InheritSettingsFrom(other storage.Factory) {
	if otherFactory, ok := other.(*Factory); ok {
		f.coreFactory.GetConfig().ApplyDefaults(otherFactory.coreFactory.GetConfig())
	}
}

func (f *Factory) IsArchiveCapable() bool {
	return f.Options.Config.namespace == archiveNamespace && f.Options.Config.Enabled
}

func (f *Factory) createTemplates(writer *esSpanStore.SpanWriterV1, cfg *config.Configuration) error {
	// Creating a template here would conflict with the one created for ILM resulting to no index rollover
	if cfg.CreateIndexTemplates && !cfg.UseILM {
		spanMapping, serviceMapping, err := f.coreFactory.GetSpanServiceMapping()
		if err != nil {
			return err
		}
		if err := writer.CreateTemplates(spanMapping, serviceMapping, cfg.Indices.IndexPrefix); err != nil {
			return err
		}
	}
	return nil
}
