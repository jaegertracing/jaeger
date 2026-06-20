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

	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/extension/extensionauth"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/fswatcher"
	"github.com/jaegertracing/jaeger/internal/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/indices"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/mappings"
	essamplestore "github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/samplingstore"
	esdepstorev2 "github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/depstore"
	esspanstore "github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core"
)

var _ io.Closer = (*FactoryBase)(nil)

// FactoryBase for Elasticsearch backend.
type FactoryBase struct {
	metricsFactory metrics.Factory
	logger         *zap.Logger
	tracer         trace.TracerProvider

	newClientFn func(ctx context.Context, c *config.Configuration, logger *zap.Logger, metricsFactory metrics.Factory, httpAuth extensionauth.HTTPClient) (es.Client, error)

	config *config.Configuration

	client atomic.Pointer[es.Client]

	pwdFileWatcher *fswatcher.FSWatcher

	templateBuilder es.TemplateBuilder

	tags []string
}

func NewFactoryBase(
	ctx context.Context,
	cfg config.Configuration,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
	httpAuth extensionauth.HTTPClient,
) (*FactoryBase, error) {
	f := &FactoryBase{
		config:      &cfg,
		newClientFn: config.NewClient,
		tracer:      otel.GetTracerProvider(),
	}
	f.metricsFactory = metricsFactory
	f.logger = logger
	f.templateBuilder = es.TextTemplateBuilder{}
	tags, err := f.config.TagKeysAsFields()
	if err != nil {
		return nil, err
	}
	f.tags = tags

	client, err := f.newClientFn(ctx, f.config, logger, metricsFactory, httpAuth)
	if err != nil {
		return nil, fmt.Errorf("failed to create Elasticsearch client: %w", err)
	}
	f.client.Store(&client)

	if f.config.Authentication.BasicAuthentication.HasValue() {
		if file := f.config.Authentication.BasicAuthentication.Get().PasswordFilePath; file != "" {
			watcher, err := fswatcher.New([]string{file}, f.onPasswordChange, f.logger)
			if err != nil {
				return nil, fmt.Errorf("failed to create watcher for ES client's password: %w", err)
			}
			f.pwdFileWatcher = watcher
		}
	}

	err = f.createTemplates(ctx)
	if err != nil {
		return nil, err
	}

	return f, nil
}

func (f *FactoryBase) getClient() es.Client {
	if c := f.client.Load(); c != nil {
		return *c
	}
	return nil
}

// GetSpanReaderParams returns the SpanReaderParams which can be used to initialize the v1 and v2 readers.
func (f *FactoryBase) GetSpanReaderParams() esspanstore.SpanReaderParams {
	spanRotation, serviceRotation := f.buildRotations()
	maxSpanAge := f.config.MaxSpanAge
	// See timeRangeDesign comment in reader.go.
	// Aliases cover all data, so we use a large maxSpanAge to ensure GetTraces by ID
	// can reach any trace regardless of age.
	if f.config.UseReadWriteAliases {
		maxSpanAge = esspanstore.DawnOfTimeSpanAge
	}
	return esspanstore.SpanReaderParams{
		Client:            f.getClient,
		MaxDocCount:       f.config.MaxDocCount,
		MaxSpanAge:        maxSpanAge,
		TagDotReplacement: f.config.Tags.DotReplacement,
		Logger:            f.logger,
		Tracer:            f.tracer.Tracer("esspanstore.SpanReader"),
		SpanRotation:      spanRotation,
		ServiceRotation:   serviceRotation,
	}
}

// GetSpanWriterParams returns the SpanWriterParams which can be used to initialize the v1 and v2 writers.
func (f *FactoryBase) GetSpanWriterParams() esspanstore.SpanWriterParams {
	spanRotation, serviceRotation := f.buildRotations()
	return esspanstore.SpanWriterParams{
		Client:            f.getClient,
		AllTagsAsFields:   f.config.Tags.AllAsFields,
		TagKeysAsFields:   f.tags,
		TagDotReplacement: f.config.Tags.DotReplacement,
		Logger:            f.logger,
		MetricsFactory:    f.metricsFactory,
		ServiceCacheTTL:   f.config.ServiceCacheTTL,
		SpanRotation:      spanRotation,
		ServiceRotation:   serviceRotation,
	}
}

// GetDependencyStoreParams returns the esdepstorev2.Params which can be used to initialize the v1 and v2 dependency stores.
func (f *FactoryBase) GetDependencyStoreParams() esdepstorev2.Params {
	return esdepstorev2.Params{
		Client:              f.getClient,
		Logger:              f.logger,
		IndexPrefix:         f.config.Indices.IndexPrefix,
		IndexDateLayout:     f.config.Indices.Dependencies.DateLayout,
		MaxDocCount:         f.config.MaxDocCount,
		UseReadWriteAliases: f.config.UseReadWriteAliases,
	}
}

func (f *FactoryBase) CreateSamplingStore(int /* maxBuckets */) (samplingstore.Store, error) {
	params := essamplestore.Params{
		Client:                 f.getClient,
		Logger:                 f.logger,
		IndexPrefix:            f.config.Indices.IndexPrefix,
		IndexDateLayout:        f.config.Indices.Sampling.DateLayout,
		IndexRolloverFrequency: config.RolloverFrequencyAsNegativeDuration(f.config.Indices.Sampling.RolloverFrequency),
		Lookback:               f.config.AdaptiveSamplingLookback,
		MaxDocCount:            f.config.MaxDocCount,
	}
	store := essamplestore.NewSamplingStore(params)

	if f.config.CreateIndexTemplates {
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
	basicAuth := cfg.Authentication.BasicAuthentication.Get()
	newPassword, err := loadTokenFromFile(basicAuth.PasswordFilePath)
	if err != nil {
		f.logger.Error("failed to reload password for Elasticsearch client", zap.Error(err))
		return
	}
	f.logger.Sugar().Infof("loaded new password of length %d from file", len(newPassword))
	newCfg := *cfg // copy by value
	newCfg.Authentication.BasicAuthentication = configoptional.Some(config.BasicAuthentication{
		Username:         basicAuth.Username,
		Password:         newPassword,
		PasswordFilePath: "", // avoid error that both are set
	})

	newClient, err := f.newClientFn(context.Background(), &newCfg, f.logger, mf, nil)
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

func (f *FactoryBase) buildRotations() (spanRotation, serviceRotation indices.Rotation) {
	spanPrefix := f.config.Indices.IndexPrefix.Apply("jaeger-span-")
	servicePrefix := f.config.Indices.IndexPrefix.Apply("jaeger-service-")

	type aliasConfig struct {
		explicitWrite string
		explicitRead  string
	}

	buildOne := func(prefix string, aliases aliasConfig, idxOpts config.IndexOptions) indices.Rotation {
		var r indices.Rotation
		switch {
		case aliases.explicitWrite != "" && aliases.explicitRead != "":
			r = indices.NewAliasedRotation(aliases.explicitWrite, aliases.explicitRead)
		case f.config.UseReadWriteAliases:
			writeSuffix := "write"
			if f.config.WriteAliasSuffix != "" {
				writeSuffix = f.config.WriteAliasSuffix
			}
			readSuffix := "read"
			if f.config.ReadAliasSuffix != "" {
				readSuffix = f.config.ReadAliasSuffix
			}
			r = indices.NewAliasedRotation(prefix+writeSuffix, prefix+readSuffix)
		default:
			r = indices.NewPeriodicRotation(prefix, idxOpts.DateLayout, config.RolloverFrequencyDuration(idxOpts.RolloverFrequency))
		}
		if len(f.config.RemoteReadClusters) > 0 {
			r = indices.NewRemoteClusterRotation(r, f.config.RemoteReadClusters)
		}
		r = indices.NewLoggingRotation(r, f.logger)
		return r
	}

	spanRotation = buildOne(spanPrefix, aliasConfig{
		explicitWrite: f.config.SpanWriteAlias,
		explicitRead:  f.config.SpanReadAlias,
	}, f.config.Indices.Spans)
	serviceRotation = buildOne(servicePrefix, aliasConfig{
		explicitWrite: f.config.ServiceWriteAlias,
		explicitRead:  f.config.ServiceReadAlias,
	}, f.config.Indices.Services)
	return spanRotation, serviceRotation
}

func (f *FactoryBase) createTemplates(ctx context.Context) error {
	if f.config.CreateIndexTemplates {
		mappingBuilder := f.mappingBuilderFromConfig(f.config)
		spanMapping, serviceMapping, err := mappingBuilder.GetSpanServiceMappings()
		if err != nil {
			return err
		}
		jaegerSpanIdx := f.config.Indices.IndexPrefix.Apply("jaeger-span")
		jaegerServiceIdx := f.config.Indices.IndexPrefix.Apply("jaeger-service")
		_, err = f.getClient().CreateTemplate(jaegerSpanIdx).Body(spanMapping).Do(ctx)
		if err != nil {
			return fmt.Errorf("failed to create template %q: %w", jaegerSpanIdx, err)
		}
		_, err = f.getClient().CreateTemplate(jaegerServiceIdx).Body(serviceMapping).Do(ctx)
		if err != nil {
			return fmt.Errorf("failed to create template %q: %w", jaegerServiceIdx, err)
		}
	}
	return nil
}
