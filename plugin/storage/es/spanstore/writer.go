// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"fmt"
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
	spanType               = "span"
	serviceType            = "service"
	serviceCacheTTLDefault = 12 * time.Hour
	indexCacheTTLDefault   = 48 * time.Hour
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
func (s *SpanWriter) WriteSpan(_ context.Context, span *model.Span) error {
	spanIndexName, serviceIndexName := s.spanServiceIndex(span.StartTime)
	jsonSpan := s.spanConverter.FromDomainEmbedProcess(span)
	if serviceIndexName != "" {
		s.writeService(serviceIndexName, jsonSpan)
	}
	s.writeSpan(spanIndexName, jsonSpan)
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
