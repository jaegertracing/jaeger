// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/cache"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/spanstoremetrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore/internal/dbmodel"
	"github.com/jaegertracing/jaeger/pkg/es"
	cfg "github.com/jaegertracing/jaeger/pkg/es/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

const (
	spanType               = "span"
	serviceType            = "service"
	serviceCacheTTLDefault = 12 * time.Hour
	indexCacheTTLDefault   = 48 * time.Hour
)

type spanWriterMetrics struct {
	indexCreate *spanstoremetrics.WriteMetrics
}

type serviceWriter func(string, *dbmodel.Span)

// JsonSpanWriter writes json Services and Spans specific to ElasticSearch.
type JsonSpanWriter struct {
	SpanWriter
}

// NewJsonSpanWriter returns an instance of JsonSpanWriter
func NewJsonSpanWriter(p SpanWriterParams) *JsonSpanWriter {
	return &JsonSpanWriter{
		SpanWriter: *NewSpanWriter(p),
	}
}

// GetSpanAndServiceIndexFn returns SpanAndServiceIndexFn which can be used to
// fetch the span and service index names by start time of span
func (j *JsonSpanWriter) GetSpanAndServiceIndexFn() SpanAndServiceIndexFn {
	return j.spanServiceIndex
}

// SpanWriter is a wrapper around elastic.Client
type SpanWriter struct {
	client        func() es.Client
	logger        *zap.Logger
	writerMetrics spanWriterMetrics // TODO: build functions to wrap around each Do fn
	// indexCache       cache.Cache
	spanConverter    dbmodel.FromDomain
	serviceWriter    serviceWriter
	spanServiceIndex SpanAndServiceIndexFn
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
	UseReadWriteAliases bool
	WriteAliasSuffix    string
	ServiceCacheTTL     time.Duration
}

// NewSpanWriter creates a new SpanWriter for use
func NewSpanWriter(p SpanWriterParams) *SpanWriter {
	serviceCacheTTL := p.ServiceCacheTTL
	if p.ServiceCacheTTL == 0 {
		serviceCacheTTL = serviceCacheTTLDefault
	}

	writeAliasSuffix := ""
	if p.UseReadWriteAliases {
		if p.WriteAliasSuffix != "" {
			writeAliasSuffix = p.WriteAliasSuffix
		} else {
			writeAliasSuffix = "write"
		}
	}

	serviceOperationStorage := NewServiceOperationStorage(p.Client, p.Logger, serviceCacheTTL)
	return &SpanWriter{
		client: p.Client,
		logger: p.Logger,
		writerMetrics: spanWriterMetrics{
			indexCreate: spanstoremetrics.NewWriter(p.MetricsFactory, "index_create"),
		},
		serviceWriter:    serviceOperationStorage.Write,
		spanConverter:    dbmodel.NewFromDomain(p.AllTagsAsFields, p.TagKeysAsFields, p.TagDotReplacement),
		spanServiceIndex: getSpanAndServiceIndexFn(p, writeAliasSuffix),
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

// SpanAndServiceIndexFn returns names of span and service indices
type SpanAndServiceIndexFn func(spanTime time.Time) (string, string)

func getSpanAndServiceIndexFn(p SpanWriterParams, writeAlias string) SpanAndServiceIndexFn {
	spanIndexPrefix := p.IndexPrefix.Apply(spanIndexBaseName)
	serviceIndexPrefix := p.IndexPrefix.Apply(serviceIndexBaseName)
	if p.UseReadWriteAliases {
		return func(_ time.Time) (string, string) {
			return spanIndexPrefix + writeAlias, serviceIndexPrefix + writeAlias
		}
	}
	return func(date time.Time) (string, string) {
		return indexWithDate(spanIndexPrefix, p.SpanIndex.DateLayout, date), indexWithDate(serviceIndexPrefix, p.ServiceIndex.DateLayout, date)
	}
}

// WriteJsonSpan writes a span and its corresponding service:operation in ElasticSearch
func (s *SpanWriter) WriteJsonSpan(serviceIndexName, spanIndexName string, span *dbmodel.Span) {
	if serviceIndexName != "" {
		s.writeService(serviceIndexName, span)
	}
	s.writeSpan(spanIndexName, span)
}

// WriteSpan writes a span and its corresponding service:operation in ElasticSearch
func (s *SpanWriter) WriteSpan(_ context.Context, span *model.Span) error {
	spanIndexName, serviceIndexName := s.spanServiceIndex(span.StartTime)
	jsonSpan := s.spanConverter.FromDomainEmbedProcess(span)
	s.WriteJsonSpan(serviceIndexName, spanIndexName, jsonSpan)
	s.logger.Debug("Wrote span to ES index", zap.String("index", spanIndexName))
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
