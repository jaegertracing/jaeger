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

package es

import (
	"flag"
	"fmt"
	"io"

	"github.com/spf13/viper"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/pkg/es/config"
	esDepStore "github.com/jaegertracing/jaeger/plugin/storage/es/dependencystore"
	"github.com/jaegertracing/jaeger/plugin/storage/es/mappings"
	esSpanStore "github.com/jaegertracing/jaeger/plugin/storage/es/spanstore"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	primaryNamespace = "es"
	archiveNamespace = "es-archive"
)

// Factory implements storage.Factory for Elasticsearch backend.
type Factory struct {
	Options *Options

	metricsFactory metrics.Factory
	logger         *zap.Logger

	primaryConfig config.ClientBuilder
	primaryClient es.Client
	archiveConfig config.ClientBuilder
	archiveClient es.Client
}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	return &Factory{
		Options: NewOptions(primaryNamespace, archiveNamespace),
	}
}

// AddFlags implements plugin.Configurable
func (f *Factory) AddFlags(flagSet *flag.FlagSet) {
	f.Options.AddFlags(flagSet)
}

// InitFromViper implements plugin.Configurable
func (f *Factory) InitFromViper(v *viper.Viper, logger *zap.Logger) {
	f.Options.InitFromViper(v)
	f.primaryConfig = f.Options.GetPrimary()
	f.archiveConfig = f.Options.Get(archiveNamespace)
}

// InitFromOptions configures factory from Options struct.
func (f *Factory) InitFromOptions(o Options) {
	f.Options = &o
	f.primaryConfig = f.Options.GetPrimary()
	if cfg := f.Options.Get(archiveNamespace); cfg != nil {
		f.archiveConfig = cfg
	}
}

// Initialize implements storage.Factory
func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	f.metricsFactory, f.logger = metricsFactory, logger

	primaryClient, err := f.primaryConfig.NewClient(logger, metricsFactory)
	if err != nil {
		return fmt.Errorf("failed to create primary Elasticsearch client: %w", err)
	}
	f.primaryClient = primaryClient
	if f.archiveConfig.IsStorageEnabled() {
		f.archiveClient, err = f.archiveConfig.NewClient(logger, metricsFactory)
		if err != nil {
			return fmt.Errorf("failed to create archive Elasticsearch client: %w", err)
		}
	}
	return nil
}

// CreateSpanReader implements storage.Factory
func (f *Factory) CreateSpanReader() (spanstore.Reader, error) {
	return createSpanReader(f.metricsFactory, f.logger, f.primaryClient, f.primaryConfig, false)
}

// CreateSpanWriter implements storage.Factory
func (f *Factory) CreateSpanWriter() (spanstore.Writer, error) {
	return createSpanWriter(f.metricsFactory, f.logger, f.primaryClient, f.primaryConfig, false)
}

// CreateDependencyReader implements storage.Factory
func (f *Factory) CreateDependencyReader() (dependencystore.Reader, error) {
	return createDependencyReader(f.logger, f.primaryClient, f.primaryConfig)
}

// CreateArchiveSpanReader implements storage.ArchiveFactory
func (f *Factory) CreateArchiveSpanReader() (spanstore.Reader, error) {
	if !f.archiveConfig.IsStorageEnabled() {
		return nil, nil
	}
	return createSpanReader(f.metricsFactory, f.logger, f.archiveClient, f.archiveConfig, true)
}

// CreateArchiveSpanWriter implements storage.ArchiveFactory
func (f *Factory) CreateArchiveSpanWriter() (spanstore.Writer, error) {
	if !f.archiveConfig.IsStorageEnabled() {
		return nil, nil
	}
	return createSpanWriter(f.metricsFactory, f.logger, f.archiveClient, f.archiveConfig, true)
}

func createSpanReader(
	mFactory metrics.Factory,
	logger *zap.Logger,
	client es.Client,
	cfg config.ClientBuilder,
	archive bool,
) (spanstore.Reader, error) {
	if cfg.GetUseILM() && !cfg.GetUseReadWriteAliases() {
		return nil, fmt.Errorf("--es.use-ilm must always be used in conjunction with --es.use-aliases to ensure ES writers and readers refer to the single index mapping")
	}
	return esSpanStore.NewSpanReader(esSpanStore.SpanReaderParams{
		Client:                        client,
		Logger:                        logger,
		MetricsFactory:                mFactory,
		MaxDocCount:                   cfg.GetMaxDocCount(),
		MaxSpanAge:                    cfg.GetMaxSpanAge(),
		IndexPrefix:                   cfg.GetIndexPrefix(),
		SpanIndexDateLayout:           cfg.GetIndexDateLayoutSpans(),
		ServiceIndexDateLayout:        cfg.GetIndexDateLayoutServices(),
		SpanIndexRolloverFrequency:    cfg.GetIndexRolloverFrequencySpansDuration(),
		ServiceIndexRolloverFrequency: cfg.GetIndexRolloverFrequencyServicesDuration(),
		TagDotReplacement:             cfg.GetTagDotReplacement(),
		UseReadWriteAliases:           cfg.GetUseReadWriteAliases(),
		Archive:                       archive,
		RemoteReadClusters:            cfg.GetRemoteReadClusters(),
	}), nil
}

func createSpanWriter(
	mFactory metrics.Factory,
	logger *zap.Logger,
	client es.Client,
	cfg config.ClientBuilder,
	archive bool,
) (spanstore.Writer, error) {
	var tags []string
	var err error
	if cfg.GetUseILM() && !cfg.GetUseReadWriteAliases() {
		return nil, fmt.Errorf("--es.use-ilm must always be used in conjunction with --es.use-aliases to ensure ES writers and readers refer to the single index mapping")
	}
	if tags, err = cfg.TagKeysAsFields(); err != nil {
		logger.Error("failed to get tag keys", zap.Error(err))
		return nil, err
	}

	mappingBuilder := mappings.MappingBuilder{
		TemplateBuilder: es.TextTemplateBuilder{},
		Shards:          cfg.GetNumShards(),
		Replicas:        cfg.GetNumReplicas(),
		EsVersion:       cfg.GetVersion(),
		IndexPrefix:     cfg.GetIndexPrefix(),
		UseILM:          cfg.GetUseILM(),
	}

	spanMapping, serviceMapping, err := mappingBuilder.GetSpanServiceMappings()
	if err != nil {
		return nil, err
	}
	writer := esSpanStore.NewSpanWriter(esSpanStore.SpanWriterParams{
		Client:                 client,
		Logger:                 logger,
		MetricsFactory:         mFactory,
		IndexPrefix:            cfg.GetIndexPrefix(),
		SpanIndexDateLayout:    cfg.GetIndexDateLayoutSpans(),
		ServiceIndexDateLayout: cfg.GetIndexDateLayoutServices(),
		AllTagsAsFields:        cfg.GetAllTagsAsFields(),
		TagKeysAsFields:        tags,
		TagDotReplacement:      cfg.GetTagDotReplacement(),
		Archive:                archive,
		UseReadWriteAliases:    cfg.GetUseReadWriteAliases(),
	})
	if cfg.IsCreateIndexTemplates() {
		err := writer.CreateTemplates(spanMapping, serviceMapping, cfg.GetIndexPrefix())
		if err != nil {
			return nil, err
		}
	}
	return writer, nil
}

func createDependencyReader(
	logger *zap.Logger,
	client es.Client,
	cfg config.ClientBuilder,
) (dependencystore.Reader, error) {

	reader := esDepStore.NewDependencyStore(esDepStore.DependencyStoreParams{
		Client:              client,
		Logger:              logger,
		IndexPrefix:         cfg.GetIndexPrefix(),
		IndexDateLayout:     cfg.GetIndexDateLayoutDependencies(),
		MaxDocCount:         cfg.GetMaxDocCount(),
		UseReadWriteAliases: cfg.GetUseReadWriteAliases(),
	})
	return reader, nil
}

var _ io.Closer = (*Factory)(nil)

// Close closes the resources held by the factory
func (f *Factory) Close() error {
	if cfg := f.Options.Get(archiveNamespace); cfg != nil {
		cfg.TLS.Close()
	}
	return f.Options.GetPrimary().TLS.Close()
}
