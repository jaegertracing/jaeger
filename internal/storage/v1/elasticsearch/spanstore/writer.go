// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/cache"
	"github.com/jaegertracing/jaeger/internal/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	cfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/spanstoremetrics"
)

const (
	spanType               = "span"
	serviceType            = "service"
	serviceCacheTTLDefault = 12 * time.Hour
	indexCacheTTLDefault   = 48 * time.Hour
)

type serviceWriter func(string, *dbmodel.Span)

// SpanWriter is a wrapper around elastic.Client
type SpanWriter struct {
	client func() es.Client
	logger *zap.Logger
	// indexCache       cache.Cache
	writerMetrics    *spanstoremetrics.WriteMetrics
	serviceWriter     serviceWriter
	spanServiceIndex  spanAndServiceIndexFn
	allTagsAsFields   bool
	tagDotReplacement string
	tagKeysAsFields   map[string]bool
}

// CoreSpanWriter is a DB-Level abstraction which directly deals with database level operations
type CoreSpanWriter interface {
	// CreateTemplates creates index templates.
	CreateTemplates(spanTemplate, serviceTemplate string, indexPrefix cfg.IndexPrefix) error
	// WriteSpan writes a span and its corresponding service:operation in ElasticSearch
	WriteSpan(spanStartTime time.Time, span *dbmodel.Span) error
	// Close closes CoreSpanWriter
	Close() error
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

	tags := map[string]bool{}
	for _, k := range p.TagKeysAsFields {
		tags[k] = true
	}

	serviceOperationStorage := NewServiceOperationStorage(p.Client, p.Logger, serviceCacheTTL)
	return &SpanWriter{
		client:           p.Client,
		logger:           p.Logger,
		writerMetrics:    spanstoremetrics.NewWriter(p.MetricsFactory, "span_write"),
		serviceWriter:     serviceOperationStorage.Write,
		spanServiceIndex:  getSpanAndServiceIndexFn(p, writeAliasSuffix),
		tagKeysAsFields:   tags,
		allTagsAsFields:   p.AllTagsAsFields,
		tagDotReplacement: p.TagDotReplacement,
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

func getSpanAndServiceIndexFn(p SpanWriterParams, writeAlias string) spanAndServiceIndexFn {
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

// WriteSpan writes a span and its corresponding service:operation in ElasticSearch
func (s *SpanWriter) WriteSpan(spanStartTime time.Time, span *dbmodel.Span) error {
	s.writerMetrics.Attempts.Inc(1)
	s.convertNestedTagsToFieldTags(span)
	spanIndexName, serviceIndexName := s.spanServiceIndex(spanStartTime)
	if serviceIndexName != "" {
		s.writeService(serviceIndexName, span)
	}
	s.logger.Debug("Wrote span to ES index", zap.String("index", spanIndexName))
	return s.writeSpan(spanIndexName, span)
}

func (s *SpanWriter) convertNestedTagsToFieldTags(span *dbmodel.Span) {
	processNestedTags, processFieldTags := s.splitElevatedTags(span.Process.Tags)
	span.Process.Tags = processNestedTags
	span.Process.Tag = processFieldTags
	nestedTags, fieldTags := s.splitElevatedTags(span.Tags)
	span.Tags = nestedTags
	span.Tag = fieldTags
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

func (s *SpanWriter) writeSpan(indexName string, jsonSpan *dbmodel.Span) error {
	s.client().Index().Index(indexName).Type(spanType).BodyJson(&jsonSpan).Add()
	return nil
}

func (s *SpanWriter) splitElevatedTags(keyValues []dbmodel.KeyValue) ([]dbmodel.KeyValue, map[string]any) {
	if !s.allTagsAsFields && len(s.tagKeysAsFields) == 0 {
		return keyValues, nil
	}
	var tagsMap map[string]any
	var kvs []dbmodel.KeyValue
	for _, kv := range keyValues {
		if kv.Type != dbmodel.BinaryType && (s.allTagsAsFields || s.tagKeysAsFields[kv.Key]) {
			if tagsMap == nil {
				tagsMap = map[string]any{}
			}
			tagsMap[strings.ReplaceAll(kv.Key, ".", s.tagDotReplacement)] = kv.Value
		} else {
			kvs = append(kvs, kv)
		}
	}
	if kvs == nil {
		kvs = make([]dbmodel.KeyValue, 0)
	}
	return kvs, tagsMap
}
