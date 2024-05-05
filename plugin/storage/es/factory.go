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
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/spf13/viper"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/pkg/es/config"
	"github.com/jaegertracing/jaeger/pkg/fswatcher"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/plugin"
	esDepStore "github.com/jaegertracing/jaeger/plugin/storage/es/dependencystore"
	"github.com/jaegertracing/jaeger/plugin/storage/es/mappings"
	esSampleStore "github.com/jaegertracing/jaeger/plugin/storage/es/samplingstore"
	esSpanStore "github.com/jaegertracing/jaeger/plugin/storage/es/spanstore"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/samplingstore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	primaryNamespace = "es"
	archiveNamespace = "es-archive"
)

var ( // interface comformance checks
	_ storage.Factory        = (*Factory)(nil)
	_ storage.ArchiveFactory = (*Factory)(nil)
	_ io.Closer              = (*Factory)(nil)
	_ plugin.Configurable    = (*Factory)(nil)
	_ storage.Purger         = (*Factory)(nil)
)

// Factory implements storage.Factory for Elasticsearch backend.
type Factory struct {
	Options *Options

	metricsFactory metrics.Factory
	logger         *zap.Logger
	tracer         trace.TracerProvider

	newClientFn func(c *config.Configuration, logger *zap.Logger, metricsFactory metrics.Factory) (es.Client, error)

	primaryConfig *config.Configuration
	archiveConfig *config.Configuration

	primaryClient atomic.Pointer[es.Client]
	archiveClient atomic.Pointer[es.Client]

	watchers []*fswatcher.FSWatcher
}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	return &Factory{
		Options:     NewOptions(primaryNamespace, archiveNamespace),
		newClientFn: config.NewClient,
		tracer:      otel.GetTracerProvider(),
	}
}

func NewFactoryWithConfig(
	cfg config.Configuration,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
) (*Factory, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	defaultConfig := getDefaultConfig()
	cfg.ApplyDefaults(&defaultConfig)

	archive := make(map[string]*namespaceConfig)
	archive[archiveNamespace] = &namespaceConfig{
		Configuration: cfg,
		namespace:     archiveNamespace,
	}

	f := NewFactory()
	f.InitFromOptions(Options{
		Primary: namespaceConfig{
			Configuration: cfg,
			namespace:     primaryNamespace,
		},
		others: archive,
	})
	err := f.Initialize(metricsFactory, logger)
	if err != nil {
		return nil, err
	}
	return f, nil
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
	f.archiveConfig = f.Options.Get(archiveNamespace)
}

// Initialize implements storage.Factory.
func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	f.metricsFactory, f.logger = metricsFactory, logger

	primaryClient, err := f.newClientFn(f.primaryConfig, logger, metricsFactory)
	if err != nil {
		return fmt.Errorf("failed to create primary Elasticsearch client: %w", err)
	}
	f.primaryClient.Store(&primaryClient)

	if f.primaryConfig.PasswordFilePath != "" {
		primaryWatcher, err := fswatcher.New([]string{f.primaryConfig.PasswordFilePath}, f.onPrimaryPasswordChange, f.logger)
		if err != nil {
			return fmt.Errorf("failed to create watcher for primary ES client's password: %w", err)
		}
		f.watchers = append(f.watchers, primaryWatcher)
	}

	if f.archiveConfig.Enabled {
		archiveClient, err := f.newClientFn(f.archiveConfig, logger, metricsFactory)
		if err != nil {
			return fmt.Errorf("failed to create archive Elasticsearch client: %w", err)
		}
		f.archiveClient.Store(&archiveClient)

		if f.archiveConfig.PasswordFilePath != "" {
			archiveWatcher, err := fswatcher.New([]string{f.archiveConfig.PasswordFilePath}, f.onArchivePasswordChange, f.logger)
			if err != nil {
				return fmt.Errorf("failed to create watcher for archive ES client's password: %w", err)
			}
			f.watchers = append(f.watchers, archiveWatcher)
		}
	}

	return nil
}

func (f *Factory) getPrimaryClient() es.Client {
	if c := f.primaryClient.Load(); c != nil {
		return *c
	}
	return nil
}

func (f *Factory) getArchiveClient() es.Client {
	if c := f.archiveClient.Load(); c != nil {
		return *c
	}
	return nil
}

// CreateSpanReader implements storage.Factory
func (f *Factory) CreateSpanReader() (spanstore.Reader, error) {
	return createSpanReader(f.getPrimaryClient, f.primaryConfig, false, f.metricsFactory, f.logger, f.tracer)
}

// CreateSpanWriter implements storage.Factory
func (f *Factory) CreateSpanWriter() (spanstore.Writer, error) {
	return createSpanWriter(f.getPrimaryClient, f.primaryConfig, false, f.metricsFactory, f.logger)
}

// CreateDependencyReader implements storage.Factory
func (f *Factory) CreateDependencyReader() (dependencystore.Reader, error) {
	return createDependencyReader(f.getPrimaryClient, f.primaryConfig, f.logger)
}

// CreateArchiveSpanReader implements storage.ArchiveFactory
func (f *Factory) CreateArchiveSpanReader() (spanstore.Reader, error) {
	if !f.archiveConfig.Enabled {
		return nil, nil
	}
	return createSpanReader(f.getArchiveClient, f.archiveConfig, true, f.metricsFactory, f.logger, f.tracer)
}

// CreateArchiveSpanWriter implements storage.ArchiveFactory
func (f *Factory) CreateArchiveSpanWriter() (spanstore.Writer, error) {
	if !f.archiveConfig.Enabled {
		return nil, nil
	}
	return createSpanWriter(f.getArchiveClient, f.archiveConfig, true, f.metricsFactory, f.logger)
}

func createSpanReader(
	clientFn func() es.Client,
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
		Client:                        clientFn,
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
	clientFn func() es.Client,
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

	writer := esSpanStore.NewSpanWriter(esSpanStore.SpanWriterParams{
		Client:                 clientFn,
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
		ServiceCacheTTL:        cfg.ServiceCacheTTL,
	})

	// Creating a template here would conflict with the one created for ILM resulting to no index rollover
	if cfg.CreateIndexTemplates && !cfg.UseILM {
		mappingBuilder := mappingBuilderFromConfig(cfg)
		spanMapping, serviceMapping, err := mappingBuilder.GetSpanServiceMappings()
		if err != nil {
			return nil, err
		}
		if err := writer.CreateTemplates(spanMapping, serviceMapping, cfg.IndexPrefix); err != nil {
			return nil, err
		}
	}
	return writer, nil
}

func (f *Factory) CreateSamplingStore(maxBuckets int) (samplingstore.Store, error) {
	params := esSampleStore.SamplingStoreParams{
		Client:                 f.getPrimaryClient,
		Logger:                 f.logger,
		IndexPrefix:            f.primaryConfig.IndexPrefix,
		IndexDateLayout:        f.primaryConfig.IndexDateLayoutSampling,
		IndexRolloverFrequency: f.primaryConfig.GetIndexRolloverFrequencySamplingDuration(),
		Lookback:               f.primaryConfig.AdaptiveSamplingLookback,
		MaxDocCount:            f.primaryConfig.MaxDocCount,
	}
	store := esSampleStore.NewSamplingStore(params)

	if f.primaryConfig.CreateIndexTemplates && !f.primaryConfig.UseILM {
		mappingBuilder := mappingBuilderFromConfig(f.primaryConfig)
		samplingMapping, err := mappingBuilder.GetSamplingMappings()
		if err != nil {
			return nil, err
		}
		if _, err := f.getPrimaryClient().CreateTemplate(params.PrefixedIndexName()).Body(samplingMapping).Do(context.Background()); err != nil {
			return nil, fmt.Errorf("failed to create template: %w", err)
		}
	}

	return store, nil
}

func mappingBuilderFromConfig(cfg *config.Configuration) mappings.MappingBuilder {
	return mappings.MappingBuilder{
		TemplateBuilder:              es.TextTemplateBuilder{},
		Shards:                       cfg.NumShards,
		Replicas:                     cfg.NumReplicas,
		EsVersion:                    cfg.Version,
		IndexPrefix:                  cfg.IndexPrefix,
		UseILM:                       cfg.UseILM,
		PrioritySpanTemplate:         cfg.PrioritySpanTemplate,
		PriorityServiceTemplate:      cfg.PriorityServiceTemplate,
		PriorityDependenciesTemplate: cfg.PriorityDependenciesTemplate,
	}
}

func createDependencyReader(
	clientFn func() es.Client,
	cfg *config.Configuration,
	logger *zap.Logger,
) (dependencystore.Reader, error) {
	reader := esDepStore.NewDependencyStore(esDepStore.DependencyStoreParams{
		Client:              clientFn,
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
	var errs []error

	for _, w := range f.watchers {
		errs = append(errs, w.Close())
	}
	if cfg := f.Options.Get(archiveNamespace); cfg != nil {
		errs = append(errs, cfg.TLS.Close())
	}
	errs = append(errs, f.Options.GetPrimary().TLS.Close())
	errs = append(errs, f.getPrimaryClient().Close())
	if client := f.getArchiveClient(); client != nil {
		errs = append(errs, client.Close())
	}

	return errors.Join(errs...)
}

func (f *Factory) onPrimaryPasswordChange() {
	f.onClientPasswordChange(f.primaryConfig, &f.primaryClient)
}

func (f *Factory) onArchivePasswordChange() {
	f.onClientPasswordChange(f.archiveConfig, &f.archiveClient)
}

func (f *Factory) onClientPasswordChange(cfg *config.Configuration, client *atomic.Pointer[es.Client]) {
	newPassword, err := loadTokenFromFile(cfg.PasswordFilePath)
	if err != nil {
		f.logger.Error("failed to reload password for Elasticsearch client", zap.Error(err))
		return
	}
	f.logger.Sugar().Infof("loaded new password of length %d from file", len(newPassword))
	newCfg := *cfg // copy by value
	newCfg.Password = newPassword
	newCfg.PasswordFilePath = "" // avoid error that both are set

	newClient, err := f.newClientFn(&newCfg, f.logger, f.metricsFactory)
	if err != nil {
		f.logger.Error("failed to recreate Elasticsearch client with new password", zap.Error(err))
		return
	}
	if oldClient := *client.Swap(&newClient); oldClient != nil {
		if err := oldClient.Close(); err != nil {
			f.logger.Error("failed to close Elasticsearch client", zap.Error(err))
		}
	}
}

func (f *Factory) Purge(ctx context.Context) error {
	esClient := f.getPrimaryClient()
	_, err := esClient.DeleteIndex("*").Do(ctx)
	return err
}

func loadTokenFromFile(path string) (string, error) {
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(b), "\r\n"), nil
}
