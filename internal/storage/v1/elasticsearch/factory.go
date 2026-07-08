// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"errors"
	"fmt"
	"io"

	"go.opentelemetry.io/collector/extension/extensionauth"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/indices"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore"
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

	// newESClientFn constructs the shared esclient over the transport pool that
	// backs every data-plane path. It is a seam so tests can inject a client that
	// doesn't probe a live cluster (esclient.NewClient issues a GET / at construction).
	newESClientFn func(ctx context.Context, c *config.Configuration, logger *zap.Logger, httpAuth extensionauth.HTTPClient) (*esclient.Client, error)
	// newBulkIndexerFn constructs the bulk writer over the esclient. It is a seam,
	// like the client constructors above, so tests can inject a failing indexer to
	// exercise the construction error path.
	newBulkIndexerFn func(client *esclient.Client, cfg esclient.BulkIndexerConfig, mf metrics.Factory, logger *zap.Logger) (*esclient.BulkIndexer, error)

	config *config.Configuration

	// esClient is the shared esclient over the transport pool that backs every
	// data-plane path; searcher and bulkWriter compose over it, and the admin
	// operations (templates, purge) run an IndicesClient over it too.
	esClient *esclient.Client
	// searcher and bulkWriter are the data-plane surfaces over the esclient
	// transport pool: service/operation reads, span writes, sampling reads/writes,
	// dependency and metric reads. The factory owns the bulk indexer's lifecycle
	// and closes it in Close.
	searcher   esclient.Searcher
	bulkWriter *esclient.BulkIndexer

	tags []string
}

// factoryOption overrides a factory field before construction proceeds. It lets
// tests inject failing/fake client constructors through the newESClientFn /
// newBulkIndexerFn seams to exercise the construction error paths.
type factoryOption func(*FactoryBase)

func NewFactoryBase(
	ctx context.Context,
	cfg config.Configuration,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
	httpAuth extensionauth.HTTPClient,
	opts ...factoryOption,
) (*FactoryBase, error) {
	f := &FactoryBase{
		config:           &cfg,
		newESClientFn:    esclient.NewClient,
		newBulkIndexerFn: esclient.NewBulkIndexer,
		tracer:           otel.GetTracerProvider(),
	}
	for _, opt := range opts {
		opt(f)
	}
	// If construction fails partway, close whatever was already created (the
	// esclient and the bulk indexer's workers). Close is nil-safe.
	success := false
	defer func() { //nolint:contextcheck // Close releases resources and takes no context
		if !success {
			_ = f.Close()
		}
	}()
	f.metricsFactory = metricsFactory
	f.logger = logger
	f.config.LogDeprecationWarnings(logger)
	tags, err := f.config.TagKeysAsFields()
	if err != nil {
		return nil, err
	}
	f.tags = tags

	// One esclient over the transport pool backs every path — the searcher, the
	// bulk indexer, and the admin IndicesClient (templates, purge) — with a single
	// version probe.
	esClient, err := f.newESClientFn(ctx, f.config, logger, httpAuth)
	if err != nil {
		return nil, fmt.Errorf("failed to create Elasticsearch data client: %w", err)
	}
	f.esClient = esClient
	f.searcher = esclient.SearchClient{Client: esClient}
	// esutil.BulkIndexer flushes on a byte threshold or a time interval only; it
	// has no action-count trigger, so BulkProcessing.MaxActions is not wired here.
	bulkWriter, err := f.newBulkIndexerFn(esClient, esclient.BulkIndexerConfig{
		FlushBytes:    f.config.BulkProcessing.MaxBytes,
		FlushInterval: f.config.BulkProcessing.FlushInterval,
		Workers:       f.config.BulkProcessing.Workers,
	}, metricsFactory, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create Elasticsearch bulk indexer: %w", err)
	}
	f.bulkWriter = bulkWriter

	err = f.createTemplates(ctx)
	if err != nil {
		return nil, err
	}

	success = true
	return f, nil
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
		Searcher:          f.searcher,
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
		BulkWriter:        f.bulkWriter,
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
		Searcher:    f.searcher,
		BulkWriter:  f.bulkWriter,
		Logger:      f.logger,
		MaxDocCount: f.config.MaxDocCount,
		Rotation:    f.buildDependencyRotation(),
	}
}

func (f *FactoryBase) CreateSamplingStore(int /* maxBuckets */) (samplingstore.Store, error) {
	params := essamplestore.Params{
		Searcher:    f.searcher,
		BulkWriter:  f.bulkWriter,
		IndexClient: &esclient.IndicesClient{Client: f.esClient},
		Logger:      f.logger,
		Lookback:    f.config.AdaptiveSamplingLookback,
		MaxDocCount: f.config.MaxDocCount,
		Rotation:    f.buildSamplingRotation(),
	}
	store := essamplestore.NewSamplingStore(params)

	if f.config.CreateIndexTemplates {
		templateName := f.config.Indices.IndexPrefix.Apply(config.SamplingIndexName)
		if err := f.indicesClient().CreateTemplate(context.Background(), templateName, esclient.SamplingMapping); err != nil {
			return nil, fmt.Errorf("failed to create template: %w", err)
		}
	}

	return store, nil
}

// indicesClient builds the esclient admin client used to install index
// templates, carrying the resolved index and ILM config the renderer needs.
// The client renders each template body from its own resolved backend version,
// so the factory no longer reads GetVersion here.
func (f *FactoryBase) indicesClient() *esclient.IndicesClient {
	spanRC := f.config.ResolvedSpanRotation()
	var ilmPolicyName string
	if spanRC.AutoRollover.HasValue() {
		ilmPolicyName = spanRC.AutoRollover.Get().PolicyName
	}
	return &esclient.IndicesClient{
		Client:  f.esClient,
		Indices: f.config.Indices,
		// Purge deletes "*" for cleanup, so tolerate missing indices rather than
		// failing when there is nothing to delete.
		IgnoreUnavailableIndex: true,
		UseILM:                 ilmPolicyName != "",
		ILMPolicyName:          ilmPolicyName,
	}
}

// Close closes the resources held by the factory. The bulk indexer is closed
// here (flushing buffered writes and stopping its workers) even when no writer
// was created, e.g. a query-only service.
func (f *FactoryBase) Close() error {
	var errs []error
	if f.bulkWriter != nil {
		errs = append(errs, f.bulkWriter.Close())
	}
	// Release the owned esclient's pooled idle connections. The data plane
	// (searcher, bulk indexer, admin ops) runs over this client. Close is safe on
	// a nil Client.
	errs = append(errs, f.esClient.Close())
	return errors.Join(errs...)
}

func (f *FactoryBase) Purge(ctx context.Context) error {
	return f.indicesClient().DeleteAllIndices(ctx)
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
	if !f.config.CreateIndexTemplates {
		return nil
	}
	ic := f.indicesClient()
	jaegerSpanIdx := f.config.Indices.IndexPrefix.Apply(config.SpanIndexName)
	jaegerServiceIdx := f.config.Indices.IndexPrefix.Apply(config.ServiceIndexName)
	if err := ic.CreateTemplate(ctx, jaegerSpanIdx, esclient.SpanMapping); err != nil {
		return fmt.Errorf("failed to create template %q: %w", jaegerSpanIdx, err)
	}
	if err := ic.CreateTemplate(ctx, jaegerServiceIdx, esclient.ServiceMapping); err != nil {
		return fmt.Errorf("failed to create template %q: %w", jaegerServiceIdx, err)
	}
	return nil
}
