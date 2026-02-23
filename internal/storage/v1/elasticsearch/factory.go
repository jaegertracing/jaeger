// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"encoding/json"
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
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/mappings"
	essamplestore "github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/samplingstore"
	esdepstorev2 "github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/depstore"
	esspanstore "github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core"
)

const traceSummaryVersion = "v1"

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

	authenticator extensionauth.HTTPClient
}

// TransformConfigResponse represents the JSON structure returned by the ES _transform API.
type transformConfigResponse struct {
	Transforms []struct {
		Description string `json:"description"`
		Dest        struct {
			Index string `json:"index"`
		} `json:"dest"`
	} `json:"transforms"`
}

func NewFactoryBase(
	ctx context.Context,
	cfg config.Configuration,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
	httpAuth extensionauth.HTTPClient,
) (*FactoryBase, error) {
	f := &FactoryBase{
		config:        &cfg,
		newClientFn:   config.NewClient,
		tracer:        otel.GetTracerProvider(),
		authenticator: httpAuth,
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

	err = f.createTraceSummaryTransform(ctx)
	if err != nil {
		f.logger.Warn(
			"Failed to provision trace summary transform; optimization will fall back to global search until transform is available",
			zap.Error(err),
		)
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
	return esspanstore.SpanReaderParams{
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
		SpanReadAlias:       f.config.SpanReadAlias,
		ServiceReadAlias:    f.config.ServiceReadAlias,
		Logger:              f.logger,
		Tracer:              f.tracer.Tracer("esspanstore.SpanReader"),
	}
}

// GetSpanWriterParams returns the SpanWriterParams which can be used to initialize the v1 and v2 writers.
func (f *FactoryBase) GetSpanWriterParams() esspanstore.SpanWriterParams {
	return esspanstore.SpanWriterParams{
		Client:              f.getClient,
		IndexPrefix:         f.config.Indices.IndexPrefix,
		SpanIndex:           f.config.Indices.Spans,
		ServiceIndex:        f.config.Indices.Services,
		AllTagsAsFields:     f.config.Tags.AllAsFields,
		TagKeysAsFields:     f.tags,
		TagDotReplacement:   f.config.Tags.DotReplacement,
		UseReadWriteAliases: f.config.UseReadWriteAliases,
		WriteAliasSuffix:    f.config.WriteAliasSuffix,
		SpanWriteAlias:      f.config.SpanWriteAlias,
		ServiceWriteAlias:   f.config.ServiceWriteAlias,
		Logger:              f.logger,
		MetricsFactory:      f.metricsFactory,
		ServiceCacheTTL:     f.config.ServiceCacheTTL,
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

func (f *FactoryBase) resolveTransformNames() (indexPrefix string, summaryIndex string, transformID string) {
	indexPrefix = strings.TrimSuffix(string(f.config.Indices.IndexPrefix), "-")
	summaryIndex = f.config.Indices.IndexPrefix.Apply("jaeger-trace-summary")
	transformID = fmt.Sprintf("%s_%s", summaryIndex, "jaeger_trace_summary_job")
	return indexPrefix, summaryIndex, transformID
}

func (f *FactoryBase) checkTransformStatus(ctx context.Context, esClient es.Client, transformID, summaryIndex string) (bool, error) {
	bodyBytes, err := esClient.Transform().Get(ctx, transformID)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return true, nil // Doesn't exist, needs creation
		}
		return false, fmt.Errorf("failed to check transform status: %w", err)
	}

	var existingConfig transformConfigResponse
	if err := json.Unmarshal(bodyBytes, &existingConfig); err == nil && len(existingConfig.Transforms) > 0 {
		t := existingConfig.Transforms[0]
		expectedDescription := "Jaeger Trace Summary - " + traceSummaryVersion
		if t.Dest.Index == summaryIndex && t.Description == expectedDescription {
			return false, nil // Exists and is up-to-date, do not create
		}
		f.logger.Info("Transform version mismatch or config change. Recreating...", zap.String("id", transformID))
		if err := esClient.Transform().Delete(ctx, transformID); err != nil {
			// strings.Contains is used here intentionally: the elastic client wraps HTTP errors
			// in a string message and does not expose a typed status code at this abstraction level.
			// A structured error type in TransformService.Get is tracked as a follow-up improvement.
			if strings.Contains(err.Error(), "404") {
				return true, nil // Doesn't exist, needs creation
			}
			return false, fmt.Errorf("failed to delete outdated transform %q: %w", transformID, err)
		}
		return true, nil // Needs to be recreated
	}
	return false, fmt.Errorf("failed to parse transform check response: %s", string(bodyBytes))
}

func (f *FactoryBase) putTransformJob(ctx context.Context, esClient es.Client, transformID, indexPrefix, summaryIndex string) error {
	mappingBuilder := f.mappingBuilderFromConfig(f.config)

	transformPayload, err := mappingBuilder.GetTraceSummaryTransform(indexPrefix, summaryIndex, traceSummaryVersion)
	if err != nil {
		return fmt.Errorf("failed to render transform template: %w", err)
	}

	if err := esClient.Transform().Put(ctx, transformID, transformPayload); err != nil {
		return fmt.Errorf("failed to create transform job: %w", err)
	}
	return nil
}

func (f *FactoryBase) createTraceSummaryTransform(ctx context.Context) error {
	indexPrefix, summaryIndex, transformID := f.resolveTransformNames()
	esClient := f.getClient()

	shouldCreate, err := f.checkTransformStatus(ctx, esClient, transformID, summaryIndex)
	if err != nil {
		return err
	}

	if shouldCreate {
		if err := f.putTransformJob(ctx, esClient, transformID, indexPrefix, summaryIndex); err != nil {
			return err
		}
	}

	return esClient.Transform().Start(ctx, transformID)
}
