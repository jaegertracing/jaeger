// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/fswatcher"
	"github.com/jaegertracing/jaeger/internal/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/mappings"
	esSampleStore "github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/samplingstore"
	esSpanStore "github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore"
	esDepStorev2 "github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/depstore"
)

const (
	primaryNamespace = "es"
	archiveNamespace = "es-archive"
)

// FactoryBase implements storage.Factory for Elasticsearch backend.
type FactoryBase struct {
	Options *Options

	metricsFactory metrics.Factory
	logger         *zap.Logger
	tracer         trace.TracerProvider

	newClientFn func(c *config.Configuration, logger *zap.Logger, metricsFactory metrics.Factory) (es.Client, error)

	config *config.Configuration

	client atomic.Pointer[es.Client]

	pwdFileWatcher *fswatcher.FSWatcher

	templateBuilder es.TemplateBuilder
}

// NewFactoryBase creates a new FactoryBase.
func NewFactoryBase() *FactoryBase {
	return &FactoryBase{
		Options:     NewOptions(primaryNamespace),
		newClientFn: config.NewClient,
		tracer:      otel.GetTracerProvider(),
	}
}

func NewArchiveFactoryBase() *FactoryBase {
	return &FactoryBase{
		Options:     NewOptions(archiveNamespace),
		newClientFn: config.NewClient,
		tracer:      otel.GetTracerProvider(),
	}
}

func NewFactoryBaseWithConfig(
	cfg config.Configuration,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
) (*FactoryBase, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	defaultConfig := DefaultConfig()
	cfg.ApplyDefaults(&defaultConfig)

	f := &FactoryBase{
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

// Initialize implements storage.Factory.
func (f *FactoryBase) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	f.metricsFactory = metricsFactory
	f.logger = logger
	f.templateBuilder = es.TextTemplateBuilder{}

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
		f.pwdFileWatcher = watcher
	}

	if f.Options != nil && f.Options.Config.namespace == archiveNamespace {
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

func (f *FactoryBase) getClient() es.Client {
	if c := f.client.Load(); c != nil {
		return *c
	}
	return nil
}

// GetSpanReaderParams returns the SpanReaderParams which can be used to initialize the v1 and v2 readers.
func (f *FactoryBase) GetSpanReaderParams() (esSpanStore.SpanReaderParams, error) {
	if f.config.UseILM && !f.config.UseReadWriteAliases {
		return esSpanStore.SpanReaderParams{}, errors.New("--es.use-ilm must always be used in conjunction with --es.use-aliases to ensure ES writers and readers refer to the single index mapping")
	}
	return esSpanStore.SpanReaderParams{
		Client:              f.getClient,
		MaxDocCount:         f.config.MaxDocCount,
		MaxSpanAge:          f.config.MaxSpanAge,
		IndexPrefix:         f.config.Indices.IndexPrefix,
		SpanIndex:           f.config.Indices.Spans,
		ServiceIndex:        f.config.Indices.Services,
		TagDotReplacement:   f.config.Tags.DotReplacement,
		UseReadWriteAliases: f.config.UseReadWriteAliases,
		ReadAliasSuffix:     f.config.ReadAliasSuffix,
		RemoteReadClusters:  f.config.RemoteReadClusters,
		Logger:              f.logger,
		Tracer:              f.tracer.Tracer("esSpanStore.SpanReader"),
	}, nil
}

// GetSpanWriterParams returns the SpanWriterParams which can be used to initialize the v1 and v2 writers.
func (f *FactoryBase) GetSpanWriterParams() (esSpanStore.SpanWriterParams, error) {
	if f.config.UseILM && !f.config.UseReadWriteAliases {
		return esSpanStore.SpanWriterParams{}, errors.New("--es.use-ilm must always be used in conjunction with --es.use-aliases to ensure ES writers and readers refer to the single index mapping")
	}
	var err error
	var tags []string
	if tags, err = f.config.TagKeysAsFields(); err != nil {
		f.logger.Error("failed to get tag keys", zap.Error(err))
		return esSpanStore.SpanWriterParams{}, err
	}
	return esSpanStore.SpanWriterParams{
		Client:              f.getClient,
		IndexPrefix:         f.config.Indices.IndexPrefix,
		SpanIndex:           f.config.Indices.Spans,
		ServiceIndex:        f.config.Indices.Services,
		AllTagsAsFields:     f.config.Tags.AllAsFields,
		TagKeysAsFields:     tags,
		TagDotReplacement:   f.config.Tags.DotReplacement,
		UseReadWriteAliases: f.config.UseReadWriteAliases,
		WriteAliasSuffix:    f.config.WriteAliasSuffix,
		Logger:              f.logger,
		MetricsFactory:      f.metricsFactory,
		ServiceCacheTTL:     f.config.ServiceCacheTTL,
	}, nil
}

// GetDependencyStoreParams returns the esDepStorev2.Params which can be used to initialize the v1 and v2 dependency stores.
func (f *FactoryBase) GetDependencyStoreParams() esDepStorev2.Params {
	return esDepStorev2.Params{
		Client:              f.getClient,
		Logger:              f.logger,
		IndexPrefix:         f.config.Indices.IndexPrefix,
		IndexDateLayout:     f.config.Indices.Dependencies.DateLayout,
		MaxDocCount:         f.config.MaxDocCount,
		UseReadWriteAliases: f.config.UseReadWriteAliases,
	}
}

func (f *FactoryBase) CreateSamplingStore(int /* maxBuckets */) (samplingstore.Store, error) {
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
		mappingBuilder := f.mappingBuilderFromConfig(f.config)
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

func (f *FactoryBase) mappingBuilderFromConfig(cfg *config.Configuration) mappings.MappingBuilder {
	return mappings.MappingBuilder{
		TemplateBuilder: f.templateBuilder,
		Indices:         cfg.Indices,
		EsVersion:       cfg.Version,
		UseILM:          cfg.UseILM,
	}
}

var _ io.Closer = (*FactoryBase)(nil)

// Close closes the resources held by the factory
func (f *FactoryBase) Close() error {
	var errs []error

	if f.pwdFileWatcher != nil {
		errs = append(errs, f.pwdFileWatcher.Close())
	}
	errs = append(errs, f.getClient().Close())

	return errors.Join(errs...)
}

func (f *FactoryBase) onPasswordChange() {
	f.onClientPasswordChange(f.config, &f.client, f.metricsFactory)
}

func (f *FactoryBase) onClientPasswordChange(cfg *config.Configuration, client *atomic.Pointer[es.Client], mf metrics.Factory) {
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

func (f *FactoryBase) Purge(ctx context.Context) error {
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

func (f *FactoryBase) GetMetricsFactory() metrics.Factory {
	return f.metricsFactory
}

func (f *FactoryBase) GetConfig() *config.Configuration {
	return f.config
}

func (f *FactoryBase) SetConfig(cfg *config.Configuration) {
	f.config = cfg
}

func (f *FactoryBase) GetSpanServiceMapping() (serviceMapping string, spanMapping string, err error) {
	mappingBuilder := f.mappingBuilderFromConfig(f.config)
	spanMapping, serviceMapping, err = mappingBuilder.GetSpanServiceMappings()
	if err != nil {
		return "", "", err
	}
	return spanMapping, serviceMapping, nil
}
