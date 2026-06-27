// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"fmt"
	"io"

	"go.opentelemetry.io/collector/extension/extensionauth"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/clientbuilder"
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

	client es.Client

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
		newClientFn: clientbuilder.NewClient,
		tracer:      otel.GetTracerProvider(),
	}
	f.metricsFactory = metricsFactory
	f.logger = logger
	f.templateBuilder = es.TextTemplateBuilder{}
	f.config.LogDeprecationWarnings(logger)
	tags, err := f.config.TagKeysAsFields()
	if err != nil {
		return nil, err
	}
	f.tags = tags

	client, err := f.newClientFn(ctx, f.config, logger, metricsFactory, httpAuth)
	if err != nil {
		return nil, fmt.Errorf("failed to create Elasticsearch client: %w", err)
	}
	f.client = client

	err = f.createTemplates(ctx)
	if err != nil {
		return nil, err
	}

	return f, nil
}

func (f *FactoryBase) getClient() es.Client {
	return f.client
}

// GetSpanReaderParams returns the SpanReaderParams which can be used to initialize the v1 and v2 readers.
func (f *FactoryBase) GetSpanReaderParams() esspanstore.SpanReaderParams {
	spanRotation, serviceRotation := f.buildRotations()
	spanRC := f.config.ResolvedSpanRotation()
	maxSpanAge := f.config.MaxSpanAge
	// See timeRangeDesign comment in reader.go.
	// For alias-based rotation, ReadTargets ignores the time range (always returns
	// the alias), so max_span_age is irrelevant for index selection. We override it
	// to DawnOfTimeSpanAge so the time-range filter in the ES query doesn't exclude
	// old traces.
	if !spanRC.Periodic.HasValue() {
		maxSpanAge = esspanstore.DawnOfTimeSpanAge
	}
	return esspanstore.SpanReaderParams{
		Client:            f.getClient,
		MaxDocCount:       f.config.MaxDocCount,
		MaxSpanAge:        maxSpanAge,
		MaxTraceDuration:  f.config.MaxTraceDuration,
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
		Client:      f.getClient,
		Logger:      f.logger,
		MaxDocCount: f.config.MaxDocCount,
		Rotation:    f.buildDependencyRotation(),
	}
}

func (f *FactoryBase) CreateSamplingStore(int /* maxBuckets */) (samplingstore.Store, error) {
	params := essamplestore.Params{
		Client:      f.getClient,
		Logger:      f.logger,
		Lookback:    f.config.AdaptiveSamplingLookback,
		MaxDocCount: f.config.MaxDocCount,
		Rotation:    f.buildSamplingRotation(),
	}
	store := essamplestore.NewSamplingStore(params)

	if f.config.CreateIndexTemplates {
		mappingBuilder := f.mappingBuilderFromConfig(f.config)
		samplingMapping, err := mappingBuilder.GetSamplingMappings()
		if err != nil {
			return nil, err
		}
		templateName := f.config.Indices.IndexPrefix.Apply(config.SamplingIndexName)
		if _, err := f.getClient().CreateTemplate(context.Background(), templateName).Body(samplingMapping).Do(context.Background()); err != nil {
			return nil, fmt.Errorf("failed to create template: %w", err)
		}
	}

	return store, nil
}

func (f *FactoryBase) mappingBuilderFromConfig(cfg *config.Configuration) mappings.MappingBuilder {
	spanRC := cfg.ResolvedSpanRotation()
	var ilmPolicyName string
	if spanRC.AutoRollover.HasValue() {
		ilmPolicyName = spanRC.AutoRollover.Get().PolicyName
	}
	return mappings.MappingBuilder{
		TemplateBuilder: f.templateBuilder,
		Indices:         cfg.Indices,
		Version:         f.client.GetVersion(),
		UseILM:          ilmPolicyName != "",
		ILMPolicyName:   ilmPolicyName,
	}
}

// Close closes the resources held by the factory
func (f *FactoryBase) Close() error {
	if c := f.getClient(); c != nil {
		return c.Close()
	}
	return nil
}

func (f *FactoryBase) Purge(ctx context.Context) error {
	esClient := f.getClient()
	_, err := esClient.DeleteIndex(ctx, "*").Do(ctx)
	return err
}

// TODO: Support RemoteClusters for sampling via a feature flag.
func (f *FactoryBase) buildSamplingRotation() indices.Rotation {
	return indices.BuildRotation(
		f.config.Indices.IndexPrefix,
		config.SamplingIndexName,
		f.config.ResolvedSamplingRotation(),
		nil,
		f.logger,
	)
}

func (f *FactoryBase) buildDependencyRotation() indices.Rotation {
	return indices.BuildRotation(
		f.config.Indices.IndexPrefix,
		config.DependencyIndexName,
		f.config.ResolvedDependencyRotation(),
		f.config.RemoteReadClusters,
		f.logger,
	)
}

func (f *FactoryBase) buildRotations() (spanRotation, serviceRotation indices.Rotation) {
	prefix := f.config.Indices.IndexPrefix
	spanRotation = indices.BuildRotation(
		prefix,
		config.SpanIndexName,
		f.config.ResolvedSpanRotation(),
		f.config.RemoteReadClusters,
		f.logger,
	)
	serviceRotation = indices.BuildRotation(
		prefix,
		config.ServiceIndexName,
		f.config.ResolvedServiceRotation(),
		f.config.RemoteReadClusters,
		f.logger,
	)
	return spanRotation, serviceRotation
}

func (f *FactoryBase) createTemplates(ctx context.Context) error {
	if f.config.CreateIndexTemplates {
		mappingBuilder := f.mappingBuilderFromConfig(f.config)
		spanMapping, serviceMapping, err := mappingBuilder.GetSpanServiceMappings()
		if err != nil {
			return err
		}
		jaegerSpanIdx := f.config.Indices.IndexPrefix.Apply(config.SpanIndexName)
		jaegerServiceIdx := f.config.Indices.IndexPrefix.Apply(config.ServiceIndexName)
		_, err = f.getClient().CreateTemplate(ctx, jaegerSpanIdx).Body(spanMapping).Do(ctx)
		if err != nil {
			return fmt.Errorf("failed to create template %q: %w", jaegerSpanIdx, err)
		}
		_, err = f.getClient().CreateTemplate(ctx, jaegerServiceIdx).Body(serviceMapping).Do(ctx)
		if err != nil {
			return fmt.Errorf("failed to create template %q: %w", jaegerServiceIdx, err)
		}
	}
	return nil
}
