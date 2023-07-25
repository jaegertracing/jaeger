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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/pkg/es/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/plugin"
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

var (
	_ io.Closer           = (*Factory)(nil)
	_ plugin.Configurable = (*Factory)(nil)
)

// Factory implements storage.Factory for Elasticsearch backend.
type Factory struct {
	Options *Options

	metricsFactory metrics.Factory
	logger         *zap.Logger
	tracer         trace.TracerProvider

	newClientFn func(c *config.Configuration, logger *zap.Logger, metricsFactory metrics.Factory) (es.Client, error)

	primaryConfig *config.Configuration
	primaryClient es.Client
	archiveConfig *config.Configuration
	archiveClient es.Client
}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	return &Factory{
		Options:     NewOptions(primaryNamespace, archiveNamespace),
		newClientFn: config.NewClient,
		tracer:      otel.GetTracerProvider(),
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

	primaryClient, err := f.newClientFn(f.primaryConfig, logger, metricsFactory)
	if err != nil {
		return fmt.Errorf("failed to create primary Elasticsearch client: %w", err)
	}
	f.primaryClient = primaryClient
	if f.archiveConfig.Enabled {
		f.archiveClient, err = f.newClientFn(f.archiveConfig, logger, metricsFactory)
		if err != nil {
			return fmt.Errorf("failed to create archive Elasticsearch client: %w", err)
		}
	}
	return nil
}

// CreateSpanReader implements storage.Factory
func (f *Factory) CreateSpanReader() (spanstore.Reader, error) {
	return createSpanReader(f.primaryClient, f.primaryConfig, false, f.metricsFactory, f.logger, f.tracer)
}

// CreateSpanWriter implements storage.Factory
func (f *Factory) CreateSpanWriter() (spanstore.Writer, error) {
	return createSpanWriter(f.primaryClient, f.primaryConfig, false, f.metricsFactory, f.logger)
}

// CreateDependencyReader implements storage.Factory
func (f *Factory) CreateDependencyReader() (dependencystore.Reader, error) {
	return createDependencyReader(f.primaryClient, f.primaryConfig, f.logger)
}

// CreateArchiveSpanReader implements storage.ArchiveFactory
func (f *Factory) CreateArchiveSpanReader() (spanstore.Reader, error) {
	if !f.archiveConfig.Enabled {
		return nil, nil
	}
	return createSpanReader(f.archiveClient, f.archiveConfig, true, f.metricsFactory, f.logger, f.tracer)
}

// CreateArchiveSpanWriter implements storage.ArchiveFactory
func (f *Factory) CreateArchiveSpanWriter() (spanstore.Writer, error) {
	if !f.archiveConfig.Enabled {
		return nil, nil
	}
	return createSpanWriter(f.archiveClient, f.archiveConfig, true, f.metricsFactory, f.logger)
}

func createSpanReader(
	client es.Client,
	cfg *config.Configuration,
	archive bool,
	mFactory metrics.Factory,
	logger *zap.Logger,
	tp trace.TracerProvider,
) (spanstore.Reader, error) {
	if cfg.UseILM && !cfg.UseReadWriteAliases {
		return nil, fmt.Errorf("--es.use-ilm must always be used in conjunction with --es.use-aliases to ensure ES writers and readers refer to the single index mapping")
	}
	return esSpanStore.NewSpanReader(esSpanStore.SpanReaderParams{
		Client:                        client,
		MaxDocCount:                   cfg.MaxDocCount,
		MaxSpanAge:                    cfg.MaxSpanAge,
		IndexPrefix:                   cfg.IndexPrefix,
		SpanIndexDateLayout:           cfg.IndexDateLayoutSpans,
		ServiceIndexDateLayout:        cfg.IndexDateLayoutServices,
		SpanIndexRolloverFrequency:    cfg.GetIndexRolloverFrequencySpansDuration(),
		ServiceIndexRolloverFrequency: cfg.GetIndexRolloverFrequencyServicesDuration(),
		TagDotReplacement:             cfg.Tags.DotReplacement,
		UseReadWriteAliases:           cfg.UseReadWriteAliases,
		Archive:                       archive,
		RemoteReadClusters:            cfg.RemoteReadClusters,
		Logger:                        logger,
		MetricsFactory:                mFactory,
		Tracer:                        tp.Tracer("esSpanStore.SpanReader"),
	}), nil
}

func createSpanWriter(
	client es.Client,
	cfg *config.Configuration,
	archive bool,
	mFactory metrics.Factory,
	logger *zap.Logger,
) (spanstore.Writer, error) {
	var tags []string
	var err error
	if cfg.UseILM && !cfg.UseReadWriteAliases {
		return nil, fmt.Errorf("--es.use-ilm must always be used in conjunction with --es.use-aliases to ensure ES writers and readers refer to the single index mapping")
	}
	if tags, err = cfg.TagKeysAsFields(); err != nil {
		logger.Error("failed to get tag keys", zap.Error(err))
		return nil, err
	}

	mappingBuilder := mappings.MappingBuilder{
		TemplateBuilder: es.TextTemplateBuilder{},
		Shards:          cfg.NumShards,
		Replicas:        cfg.NumReplicas,
		EsVersion:       cfg.Version,
		IndexPrefix:     cfg.IndexPrefix,
		UseILM:          cfg.UseILM,
	}

	spanMapping, serviceMapping, err := mappingBuilder.GetSpanServiceMappings()
	if err != nil {
		return nil, err
	}
	writer := esSpanStore.NewSpanWriter(esSpanStore.SpanWriterParams{
		Client:                 client,
		IndexPrefix:            cfg.IndexPrefix,
		SpanIndexDateLayout:    cfg.IndexDateLayoutSpans,
		ServiceIndexDateLayout: cfg.IndexDateLayoutServices,
		AllTagsAsFields:        cfg.Tags.AllAsFields,
		TagKeysAsFields:        tags,
		TagDotReplacement:      cfg.Tags.DotReplacement,
		Archive:                archive,
		UseReadWriteAliases:    cfg.UseReadWriteAliases,
		Logger:                 logger,
		MetricsFactory:         mFactory,
	})

	// Creating a template here would conflict with the one created for ILM resulting to no index rollover
	if cfg.CreateIndexTemplates && !cfg.UseILM {
		err := writer.CreateTemplates(spanMapping, serviceMapping, cfg.IndexPrefix)
		if err != nil {
			return nil, err
		}
	}
	return writer, nil
}

func createDependencyReader(
	client es.Client,
	cfg *config.Configuration,
	logger *zap.Logger,
) (dependencystore.Reader, error) {
	reader := esDepStore.NewDependencyStore(esDepStore.DependencyStoreParams{
		Client:              client,
		Logger:              logger,
		IndexPrefix:         cfg.IndexPrefix,
		IndexDateLayout:     cfg.IndexDateLayoutDependencies,
		MaxDocCount:         cfg.MaxDocCount,
		UseReadWriteAliases: cfg.UseReadWriteAliases,
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
