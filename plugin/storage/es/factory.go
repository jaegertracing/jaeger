// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

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
	"github.com/jaegertracing/jaeger/storage/spanstore/spanstoremetrics"
)

const (
	primaryNamespace = "es"
	archiveNamespace = "es-archive"
)

var ( // interface comformance checks
	_ storage.Factory     = (*Factory)(nil)
	_ io.Closer           = (*Factory)(nil)
	_ plugin.Configurable = (*Factory)(nil)
	_ storage.Purger      = (*Factory)(nil)
)

// Factory implements storage.Factory for Elasticsearch backend.
type Factory struct {
	Options *Options

	metricsFactory metrics.Factory
	logger         *zap.Logger
	tracer         trace.TracerProvider

	newClientFn func(c *config.Configuration, logger *zap.Logger, metricsFactory metrics.Factory) (es.Client, error)

	config *config.Configuration

	client atomic.Pointer[es.Client]

	watchers []*fswatcher.FSWatcher
}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	return &Factory{
		Options:     NewOptions(primaryNamespace),
		newClientFn: config.NewClient,
		tracer:      otel.GetTracerProvider(),
	}
}

func NewArchiveFactory() *Factory {
	return &Factory{
		Options:     NewOptions(archiveNamespace),
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

	defaultConfig := DefaultConfig()
	cfg.ApplyDefaults(&defaultConfig)

	f := &Factory{
		config:      &cfg,
		newClientFn: config.NewClient,
		tracer:      otel.GetTracerProvider(),
	}
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
func (f *Factory) InitFromViper(v *viper.Viper, _ *zap.Logger) {
	f.Options.InitFromViper(v)
	f.configureFromOptions(f.Options)
}

// configureFromOptions configures factory from Options struct.
func (f *Factory) configureFromOptions(o *Options) {
	f.Options = o
	f.config = f.Options.GetConfig()
}

// Initialize implements storage.Factory.
func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	f.metricsFactory = metricsFactory
	f.logger = logger

	client, err := f.newClientFn(f.config, logger, metricsFactory)
	if err != nil {
		return fmt.Errorf("failed to create Elasticsearch client: %w", err)
	}
	f.client.Store(&client)

	if f.config.Authentication.BasicAuthentication.PasswordFilePath != "" {
		watcher, err := fswatcher.New([]string{f.config.Authentication.BasicAuthentication.PasswordFilePath}, f.onPasswordChange, f.logger)
		if err != nil {
			return fmt.Errorf("failed to create watcher for ES client's password: %w", err)
		}
		f.watchers = append(f.watchers, watcher)
	}

	if f.Options.Config.namespace == archiveNamespace {
		aliasSuffix := "archive"
		if f.config.UseReadWriteAliases {
			f.config.ReadAliasSuffix = aliasSuffix + "-read"
			f.config.WriteAliasSuffix = aliasSuffix + "-write"
		} else {
			f.config.ReadAliasSuffix = aliasSuffix
			f.config.WriteAliasSuffix = aliasSuffix
		}

		f.config.UseReadWriteAliases = true
	}

	return nil
}

func (f *Factory) getClient() es.Client {
	if c := f.client.Load(); c != nil {
		return *c
	}
	return nil
}

// CreateSpanReader implements storage.Factory
func (f *Factory) CreateSpanReader() (spanstore.Reader, error) {
	sr, err := createSpanReader(f.getClient, f.config, f.logger, f.tracer, f.config.ReadAliasSuffix, f.config.UseReadWriteAliases)
	if err != nil {
		return nil, err
	}
	return spanstoremetrics.NewReaderDecorator(sr, f.metricsFactory), nil
}

// CreateSpanWriter implements storage.Factory
func (f *Factory) CreateSpanWriter() (spanstore.Writer, error) {
	return createSpanWriter(f.getClient, f.config, f.metricsFactory, f.logger, f.config.WriteAliasSuffix, f.config.UseReadWriteAliases)
}

// CreateDependencyReader implements storage.Factory
func (f *Factory) CreateDependencyReader() (dependencystore.Reader, error) {
	return createDependencyReader(f.getClient, f.config, f.logger)
}

func createSpanReader(
	clientFn func() es.Client,
	cfg *config.Configuration,
	logger *zap.Logger,
	tp trace.TracerProvider,
	readAliasSuffix string,
	useReadWriteAliases bool,
) (spanstore.Reader, error) {
	if cfg.UseILM && !cfg.UseReadWriteAliases {
		return nil, errors.New("--es.use-ilm must always be used in conjunction with --es.use-aliases to ensure ES writers and readers refer to the single index mapping")
	}
	return esSpanStore.NewSpanReader(esSpanStore.SpanReaderParams{
		Client:              clientFn,
		MaxDocCount:         cfg.MaxDocCount,
		MaxSpanAge:          cfg.MaxSpanAge,
		IndexPrefix:         cfg.Indices.IndexPrefix,
		SpanIndex:           cfg.Indices.Spans,
		ServiceIndex:        cfg.Indices.Services,
		TagDotReplacement:   cfg.Tags.DotReplacement,
		UseReadWriteAliases: useReadWriteAliases,
		ReadAliasSuffix:     readAliasSuffix,
		RemoteReadClusters:  cfg.RemoteReadClusters,
		Logger:              logger,
		Tracer:              tp.Tracer("esSpanStore.SpanReader"),
	}), nil
}

func createSpanWriter(
	clientFn func() es.Client,
	cfg *config.Configuration,
	mFactory metrics.Factory,
	logger *zap.Logger,
	writeAliasSuffix string,
	useReadWriteAliases bool,
) (spanstore.Writer, error) {
	var tags []string
	var err error
	if cfg.UseILM && !cfg.UseReadWriteAliases {
		return nil, errors.New("--es.use-ilm must always be used in conjunction with --es.use-aliases to ensure ES writers and readers refer to the single index mapping")
	}
	if tags, err = cfg.TagKeysAsFields(); err != nil {
		logger.Error("failed to get tag keys", zap.Error(err))
		return nil, err
	}

	writer := esSpanStore.NewSpanWriter(esSpanStore.SpanWriterParams{
		Client:              clientFn,
		IndexPrefix:         cfg.Indices.IndexPrefix,
		SpanIndex:           cfg.Indices.Spans,
		ServiceIndex:        cfg.Indices.Services,
		AllTagsAsFields:     cfg.Tags.AllAsFields,
		TagKeysAsFields:     tags,
		TagDotReplacement:   cfg.Tags.DotReplacement,
		UseReadWriteAliases: useReadWriteAliases,
		WriteAliasSuffix:    writeAliasSuffix,
		Logger:              logger,
		MetricsFactory:      mFactory,
		ServiceCacheTTL:     cfg.ServiceCacheTTL,
	})

	// Creating a template here would conflict with the one created for ILM resulting to no index rollover
	if cfg.CreateIndexTemplates && !cfg.UseILM {
		mappingBuilder := mappingBuilderFromConfig(cfg)
		spanMapping, serviceMapping, err := mappingBuilder.GetSpanServiceMappings()
		if err != nil {
			return nil, err
		}
		if err := writer.CreateTemplates(spanMapping, serviceMapping, cfg.Indices.IndexPrefix); err != nil {
			return nil, err
		}
	}
	return writer, nil
}

func (f *Factory) CreateSamplingStore(int /* maxBuckets */) (samplingstore.Store, error) {
	params := esSampleStore.Params{
		Client:                 f.getClient,
		Logger:                 f.logger,
		IndexPrefix:            f.config.Indices.IndexPrefix,
		IndexDateLayout:        f.config.Indices.Sampling.DateLayout,
		IndexRolloverFrequency: config.RolloverFrequencyAsNegativeDuration(f.config.Indices.Sampling.RolloverFrequency),
		Lookback:               f.config.AdaptiveSamplingLookback,
		MaxDocCount:            f.config.MaxDocCount,
	}
	store := esSampleStore.NewSamplingStore(params)

	if f.config.CreateIndexTemplates && !f.config.UseILM {
		mappingBuilder := mappingBuilderFromConfig(f.config)
		samplingMapping, err := mappingBuilder.GetSamplingMappings()
		if err != nil {
			return nil, err
		}
		if _, err := f.getClient().CreateTemplate(params.PrefixedIndexName()).Body(samplingMapping).Do(context.Background()); err != nil {
			return nil, fmt.Errorf("failed to create template: %w", err)
		}
	}

	return store, nil
}

func mappingBuilderFromConfig(cfg *config.Configuration) mappings.MappingBuilder {
	return mappings.MappingBuilder{
		TemplateBuilder: es.TextTemplateBuilder{},
		Indices:         cfg.Indices,
		EsVersion:       cfg.Version,
		UseILM:          cfg.UseILM,
	}
}

func createDependencyReader(
	clientFn func() es.Client,
	cfg *config.Configuration,
	logger *zap.Logger,
) (dependencystore.Reader, error) {
	reader := esDepStore.NewDependencyStore(esDepStore.Params{
		Client:              clientFn,
		Logger:              logger,
		IndexPrefix:         cfg.Indices.IndexPrefix,
		IndexDateLayout:     cfg.Indices.Dependencies.DateLayout,
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
	errs = append(errs, f.getClient().Close())

	return errors.Join(errs...)
}

func (f *Factory) onPasswordChange() {
	f.onClientPasswordChange(f.config, &f.client, f.metricsFactory)
}

func (f *Factory) onClientPasswordChange(cfg *config.Configuration, client *atomic.Pointer[es.Client], mf metrics.Factory) {
	newPassword, err := loadTokenFromFile(cfg.Authentication.BasicAuthentication.PasswordFilePath)
	if err != nil {
		f.logger.Error("failed to reload password for Elasticsearch client", zap.Error(err))
		return
	}
	f.logger.Sugar().Infof("loaded new password of length %d from file", len(newPassword))
	newCfg := *cfg // copy by value
	newCfg.Authentication.BasicAuthentication.Password = newPassword
	newCfg.Authentication.BasicAuthentication.PasswordFilePath = "" // avoid error that both are set

	newClient, err := f.newClientFn(&newCfg, f.logger, mf)
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
	esClient := f.getClient()
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
