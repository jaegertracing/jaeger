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
	Options        *Options
	coreFactory    *FactoryBase
	metricsFactory metrics.Factory
}

func NewFactory() *Factory {
	return &Factory{
		Options: NewOptions(primaryNamespace),
	}
}

func NewArchiveFactory() *Factory {
	return &Factory{
		Options: NewOptions(archiveNamespace),
	}
}

func (f *Factory) AddFlags(flagSet *flag.FlagSet) {
	f.Options.AddFlags(flagSet)
}

func (f *Factory) InitFromViper(v *viper.Viper, _ *zap.Logger) {
	f.Options.InitFromViper(v)
}

func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	cfg := f.Options.GetConfig()
	if err := cfg.Validate(); err != nil {
		return err
	}
	defaultConfig := DefaultConfig()
	cfg.ApplyDefaults(&defaultConfig)
	if f.Options.Config.namespace == archiveNamespace {
		aliasSuffix := "archive"
		if cfg.UseReadWriteAliases {
			cfg.ReadAliasSuffix = aliasSuffix + "-read"
			cfg.WriteAliasSuffix = aliasSuffix + "-write"
		} else {
			cfg.ReadAliasSuffix = aliasSuffix
			cfg.WriteAliasSuffix = aliasSuffix
		}
		cfg.UseReadWriteAliases = true
	}
	coreFactory, err := NewFactoryBase(context.Background(), *cfg, metricsFactory, logger)
	if err != nil {
		return err
	}
	f.coreFactory = coreFactory
	f.metricsFactory = metricsFactory
	return nil
}

// CreateSpanReader implements storage.Factory
func (f *Factory) CreateSpanReader() (spanstore.Reader, error) {
	params := f.coreFactory.GetSpanReaderParams()
	sr := esSpanStore.NewSpanReaderV1(params)
	return spanstoremetrics.NewReaderDecorator(sr, f.metricsFactory), nil
}

// CreateSpanWriter implements storage.Factory
func (f *Factory) CreateSpanWriter() (spanstore.Writer, error) {
	params := f.coreFactory.GetSpanWriterParams()
	wr := esSpanStore.NewSpanWriterV1(params)
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
		f.getConfig().ApplyDefaults(otherFactory.getConfig())
	}
}

func (f *Factory) IsArchiveCapable() bool {
	return f.Options.Config.namespace == archiveNamespace && f.Options.Config.Configuration.Enabled
}

func (f *Factory) getConfig() *config.Configuration {
	return f.Options.GetConfig()
}
