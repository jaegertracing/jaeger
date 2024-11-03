// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/cache"
	"github.com/jaegertracing/jaeger/pkg/es"
	cfg "github.com/jaegertracing/jaeger/pkg/es/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/plugin/storage/es/spanstore/dbmodel"
	storageMetrics "github.com/jaegertracing/jaeger/storage/spanstore/metrics"
)

const (
	spanType                = "span"
	serviceType             = "service"
	serviceCacheTTLDefault  = 12 * time.Hour
	indexCacheTTLDefault    = 48 * time.Hour
	defaultIndexWaitTimeout = 60 * time.Second
)

type spanWriterMetrics struct {
	indexCreate *storageMetrics.WriteMetrics
}

type serviceWriter func(string, *dbmodel.Span)

// SpanWriter is a wrapper around elastic.Client
type SpanWriter struct {
	client        func() es.Client
	logger        *zap.Logger
	writerMetrics spanWriterMetrics // TODO: build functions to wrap around each Do fn
	// indexCache       cache.Cache
	serviceWriter    serviceWriter
	spanConverter    dbmodel.FromDomain
	spanServiceIndex spanAndServiceIndexFn
	indexCache       sync.Map
}

func (s *SpanWriter) ensureIndex(ctx context.Context, indexName string) error {
	if _, exists := s.indexCache.Load(indexName); exists {
		return nil
	}

	_, loaded := s.indexCache.LoadOrStore(indexName, struct{}{})
	if loaded {
		return nil
	}

	exists, err := s.client().IndexExists(indexName).Do(ctx)
	if err != nil {
		return fmt.Errorf("failed to check index existence: %w", err)
	}

	if !exists {
		s.logger.Info("Creating index", zap.String("index", indexName))

		// Set specific settings for the test environment
		body := `{
            "settings": {
                "number_of_shards": 1,
                "number_of_replicas": 0,
                "index.write.wait_for_active_shards": 1
            }
        }`

		_, err = s.client().CreateIndex(indexName).Body(body).Do(ctx)
		if err != nil {
			return fmt.Errorf("failed to create index with settings: %w", err)
		}
		s.logger.Info("Index created with settings",
			zap.String("index", indexName),
			zap.String("settings", body))
	}

	// Wait for index to be ready by checking its existence repeatedly
	deadline := time.Now().Add(defaultIndexWaitTimeout)
	start := time.Now()
	for time.Now().Before(deadline) {
		exists, err := s.client().IndexExists(indexName).Do(ctx)
		if err == nil && exists {
			s.logger.Info("Index is ready",
				zap.String("index", indexName),
				zap.Duration("took", time.Since(start)))
			return nil
		}
		s.logger.Debug("Waiting for index to be ready",
			zap.String("index", indexName),
			zap.Duration("elapsed", time.Since(start)))
		time.Sleep(time.Second)
	}

	return fmt.Errorf("timeout waiting for index %s to be ready", indexName)
}

// SpanWriterParams holds constructor parameters for NewSpanWriter
type SpanWriterParams struct {
	Client              func() es.Client
	Logger              *zap.Logger
	MetricsFactory      metrics.Factory
	SpanIndex           cfg.IndexOptions
	ServiceIndex        cfg.IndexOptions
	IndexPrefix         cfg.IndexPrefix
	AllTagsAsFields     bool
	TagKeysAsFields     []string
	TagDotReplacement   string
	Archive             bool
	UseReadWriteAliases bool
	ServiceCacheTTL     time.Duration
}

// NewSpanWriter creates a new SpanWriter for use
func NewSpanWriter(p SpanWriterParams) *SpanWriter {
	serviceCacheTTL := p.ServiceCacheTTL
	if p.ServiceCacheTTL == 0 {
		serviceCacheTTL = serviceCacheTTLDefault
	}

	serviceOperationStorage := NewServiceOperationStorage(p.Client, p.Logger, serviceCacheTTL)
	return &SpanWriter{
		client: p.Client,
		logger: p.Logger,
		writerMetrics: spanWriterMetrics{
			indexCreate: storageMetrics.NewWriteMetrics(p.MetricsFactory, "index_create"),
		},
		serviceWriter:    serviceOperationStorage.Write,
		spanConverter:    dbmodel.NewFromDomain(p.AllTagsAsFields, p.TagKeysAsFields, p.TagDotReplacement),
		spanServiceIndex: getSpanAndServiceIndexFn(p),
	}
}

// CreateTemplates creates index templates.
func (s *SpanWriter) CreateTemplates(spanTemplate, serviceTemplate string, indexPrefix cfg.IndexPrefix) error {
	jaegerSpanIdx := indexPrefix.Apply("jaeger-span")
	jaegerServiceIdx := indexPrefix.Apply("jaeger-service")
	_, err := s.client().CreateTemplate(jaegerSpanIdx).Body(spanTemplate).Do(context.Background())
	if err != nil {
		return fmt.Errorf("failed to create template %q: %w", jaegerSpanIdx, err)
	}
	_, err = s.client().CreateTemplate(jaegerServiceIdx).Body(serviceTemplate).Do(context.Background())
	if err != nil {
		return fmt.Errorf("failed to create template %q: %w", jaegerServiceIdx, err)
	}
	return nil
}

// spanAndServiceIndexFn returns names of span and service indices
type spanAndServiceIndexFn func(spanTime time.Time) (string, string)

func getSpanAndServiceIndexFn(p SpanWriterParams) spanAndServiceIndexFn {
	spanIndexPrefix := p.IndexPrefix.Apply(spanIndexBaseName)
	serviceIndexPrefix := p.IndexPrefix.Apply(serviceIndexBaseName)
	if p.Archive {
		return func(_ time.Time) (string, string) {
			if p.UseReadWriteAliases {
				return archiveIndex(spanIndexPrefix, archiveWriteIndexSuffix), ""
			}
			return archiveIndex(spanIndexPrefix, archiveIndexSuffix), ""
		}
	}

	if p.UseReadWriteAliases {
		return func(_ /* spanTime */ time.Time) (string, string) {
			return spanIndexPrefix + "write", serviceIndexPrefix + "write"
		}
	}
	return func(date time.Time) (string, string) {
		return indexWithDate(spanIndexPrefix, p.SpanIndex.DateLayout, date), indexWithDate(serviceIndexPrefix, p.ServiceIndex.DateLayout, date)
	}
}

// WriteSpan writes a span and its corresponding service:operation in ElasticSearch
func (s *SpanWriter) WriteSpan(ctx context.Context, span *model.Span) error {
	spanIndexName, serviceIndexName := s.spanServiceIndex(span.StartTime)

	// Ensure indices exist before writing
	if err := s.ensureIndex(ctx, spanIndexName); err != nil {
		return fmt.Errorf("failed to ensure span index: %w", err)
	}
	if serviceIndexName != "" {
		if err := s.ensureIndex(ctx, serviceIndexName); err != nil {
			return fmt.Errorf("failed to ensure service index: %w", err)
		}
	}

	jsonSpan := s.spanConverter.FromDomainEmbedProcess(span)
	if serviceIndexName != "" {
		s.writeService(serviceIndexName, jsonSpan)
	}

	// Write with retries
	var lastErr error
	for i := 0; i < 3; i++ {
		err := s.writeSpanWithResult(ctx, spanIndexName, jsonSpan)
		if err == nil {
			return nil
		}
		lastErr = err
		s.logger.Debug("Retrying span write",
			zap.String("index", spanIndexName),
			zap.Int("attempt", i+1),
			zap.Error(lastErr))
		time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
	}

	return fmt.Errorf("failed to write span after retries: %w", lastErr)
}

func (s *SpanWriter) writeSpanWithResult(_ context.Context, indexName string, jsonSpan *dbmodel.Span) error {
	indexService := s.client().Index().
		Index(indexName).
		Type(spanType).
		BodyJson(jsonSpan)

	indexService.Add()
	return nil
}

// Close closes SpanWriter
func (s *SpanWriter) Close() error {
	return s.client().Close()
}

func keyInCache(key string, c cache.Cache) bool {
	return c.Get(key) != nil
}

func writeCache(key string, c cache.Cache) {
	c.Put(key, key)
}

func (s *SpanWriter) writeService(indexName string, jsonSpan *dbmodel.Span) {
	s.serviceWriter(indexName, jsonSpan)
}

func (s *SpanWriter) writeSpan(indexName string, jsonSpan *dbmodel.Span) {
	s.client().Index().Index(indexName).Type(spanType).BodyJson(&jsonSpan).Add()
}
