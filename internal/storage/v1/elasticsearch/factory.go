// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

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

type CoreFactory interface {
	// Initialize initializes the CoreFactory.
	Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error
	// CreateSamplingStore creates samplingstore.Store
	CreateSamplingStore(int /* maxBuckets */) (samplingstore.Store, error)
	// Close closes the resources held by the factory
	Close() error
	// Purge purges the ES Storage
	Purge(ctx context.Context) error
	// GetSpanWriterParams returns esSpanStore.SpanWriterParams to initialize SpanWriter/TraceWriter
	GetSpanWriterParams() (esSpanStore.SpanWriterParams, error)
	// GetSpanReaderParams returns esSpanStore.SpanReaderParams to initialize SpanReader/TraceReader
	GetSpanReaderParams() (esSpanStore.SpanReaderParams, error)
	// GetDependencyStoreParams returns esDepStorev2.Params to initialize DependencyStore
	GetDependencyStoreParams() esDepStorev2.Params
	// GetMetricsFactory returns metrics.Factory related to CoreFactory
	GetMetricsFactory() metrics.Factory
	// IsArchiveCapable checks if storage is archive capable
	IsArchiveCapable() bool
	// GetConfig returns the config related to CoreFactory
	GetConfig() *config.Configuration
	// SetConfig sets the config related to CoreFactory
	SetConfig(config *config.Configuration)
}

// Factory implements storage.Factory for Elasticsearch backend.
type Factory struct {
	Options *Options

	metricsFactory metrics.Factory
	logger         *zap.Logger
	tracer         trace.TracerProvider

	newClientFn func(c *config.Configuration, logger *zap.Logger, metricsFactory metrics.Factory) (es.Client, error)

	config *config.Configuration

	client atomic.Pointer[es.Client]

	pwdFileWatcher *fswatcher.FSWatcher
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

// AddFlags implements storage.Configurable
func (f *Factory) AddFlags(flagSet *flag.FlagSet) {
	f.Options.AddFlags(flagSet)
}

// InitFromViper implements storage.Configurable
func (f *Factory) InitFromViper(v *viper.Viper) {
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

func (f *Factory) getClient() es.Client {
	if c := f.client.Load(); c != nil {
		return *c
	}
	return nil
}

func (f *Factory) GetSpanReaderParams() (esSpanStore.SpanReaderParams, error) {
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

func (f *Factory) GetSpanWriterParams() (esSpanStore.SpanWriterParams, error) {
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

func (f *Factory) GetDependencyStoreParams() esDepStorev2.Params {
	return esDepStorev2.Params{
		Client:              f.getClient,
		Logger:              f.logger,
		IndexPrefix:         f.config.Indices.IndexPrefix,
		IndexDateLayout:     f.config.Indices.Dependencies.DateLayout,
		MaxDocCount:         f.config.MaxDocCount,
		UseReadWriteAliases: f.config.UseReadWriteAliases,
	}
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

var _ io.Closer = (*Factory)(nil)

// Close closes the resources held by the factory
func (f *Factory) Close() error {
	var errs []error

	if f.pwdFileWatcher != nil {
		errs = append(errs, f.pwdFileWatcher.Close())
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

func (f *Factory) IsArchiveCapable() bool {
	return f.Options.Config.namespace == archiveNamespace && f.Options.Config.Enabled
}

func (f *Factory) GetMetricsFactory() metrics.Factory {
	return f.metricsFactory
}

func (f *Factory) GetConfig() *config.Configuration {
	return f.config
}

func (f *Factory) SetConfig(config *config.Configuration) {
	f.config = config
}
