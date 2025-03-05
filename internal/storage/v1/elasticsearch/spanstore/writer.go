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
	client           func() es.Client
	logger           *zap.Logger
	serviceWriter    serviceWriter
	spanServiceIndex SpanAndServiceIndexFn
	writerMetrics    spanWriterMetrics // TODO: build functions to wrap around each Do fn
}

// NewJsonSpanWriter returns JsonSpanWriter which depends only on the json structure for storing spans and services in ES/OS
func NewJsonSpanWriter(p SpanWriterParams, writeAliasSuffix string, serviceCacheTTL time.Duration) *JsonSpanWriter {
	serviceOperationStorage := NewServiceOperationStorage(p.Client, p.Logger, serviceCacheTTL)
	return &JsonSpanWriter{
		client:           p.Client,
		logger:           p.Logger,
		serviceWriter:    serviceOperationStorage.Write,
		spanServiceIndex: getSpanAndServiceIndexFn(p, writeAliasSuffix),
		writerMetrics: spanWriterMetrics{
			indexCreate: spanstoremetrics.NewWriter(p.MetricsFactory, "index_create"),
		},
	}
}

// SpanWriter is a wrapper around elastic.Client
type SpanWriter struct {
	JsonSpanWriter
	// indexCache       cache.Cache
	spanConverter dbmodel.FromDomain
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

	return &SpanWriter{
		JsonSpanWriter: *NewJsonSpanWriter(p, writeAliasSuffix, serviceCacheTTL),
		spanConverter:  dbmodel.NewFromDomain(p.AllTagsAsFields, p.TagKeysAsFields, p.TagDotReplacement),
	}
}

// CreateTemplates creates index templates.
func (j *JsonSpanWriter) CreateTemplates(spanTemplate, serviceTemplate string, indexPrefix cfg.IndexPrefix) error {
	jaegerSpanIdx := indexPrefix.Apply("jaeger-span")
	jaegerServiceIdx := indexPrefix.Apply("jaeger-service")
	_, err := j.client().CreateTemplate(jaegerSpanIdx).Body(spanTemplate).Do(context.Background())
	if err != nil {
		return fmt.Errorf("failed to create template %q: %w", jaegerSpanIdx, err)
	}
	_, err = j.client().CreateTemplate(jaegerServiceIdx).Body(serviceTemplate).Do(context.Background())
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

// GetSpanServiceIndexFn returns the SpanAndServiceIndexFn which can be used to fetch index names for spans and services.
func (j *JsonSpanWriter) GetSpanServiceIndexFn() SpanAndServiceIndexFn {
	return j.spanServiceIndex
}

// WriteJsonSpan writes a span and its corresponding service:operation in ElasticSearch
func (j *JsonSpanWriter) WriteJsonSpan(serviceIndexName, spanIndexName string, span *dbmodel.Span) {
	if serviceIndexName != "" {
		j.writeService(serviceIndexName, span)
	}
	j.writeSpan(spanIndexName, span)
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

func (j *JsonSpanWriter) writeService(indexName string, jsonSpan *dbmodel.Span) {
	j.serviceWriter(indexName, jsonSpan)
}

func (j *JsonSpanWriter) writeSpan(indexName string, jsonSpan *dbmodel.Span) {
	j.client().Index().Index(indexName).Type(spanType).BodyJson(&jsonSpan).Add()
}
