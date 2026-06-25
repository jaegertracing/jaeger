// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/cache"
	"github.com/jaegertracing/jaeger/internal/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/indices"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/shared/assembly"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/spanstoremetrics"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
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
	writerMetrics     *spanstoremetrics.WriteMetrics
	serviceWriter     serviceWriter
	spanRotation      indices.Rotation
	serviceRotation   indices.Rotation
	allTagsAsFields   bool
	tagDotReplacement string
	tagKeysAsFields   map[string]bool
}

// Writer is a DB-Level abstraction which directly deals with database level operations
type Writer interface {
	// WriteSpan writes a span and its corresponding service:operation in ElasticSearch
	WriteSpan(spanStartTime time.Time, span *dbmodel.Span)
	// Close closes Writer
	Close() error
}

// SpanWriterParams holds constructor parameters for NewSpanWriter
type SpanWriterParams struct {
	Client            func() es.Client
	Logger            *zap.Logger
	MetricsFactory    metrics.Factory
	AllTagsAsFields   bool
	TagKeysAsFields   []string
	TagDotReplacement string
	ServiceCacheTTL   time.Duration
	SpanRotation      indices.Rotation
	ServiceRotation   indices.Rotation
}

// NewSpanWriter creates a new SpanWriter for use
func NewSpanWriter(p SpanWriterParams) *SpanWriter {
	serviceCacheTTL := p.ServiceCacheTTL
	if p.ServiceCacheTTL == 0 {
		serviceCacheTTL = serviceCacheTTLDefault
	}

	tags := map[string]bool{}
	for _, k := range p.TagKeysAsFields {
		tags[k] = true
	}

	serviceOperationStorage := NewServiceOperationStorage(p.Client, p.Logger, serviceCacheTTL)
	return &SpanWriter{
		client:            p.Client,
		logger:            p.Logger,
		writerMetrics:     spanstoremetrics.NewWriter(p.MetricsFactory, "spans"),
		serviceWriter:     serviceOperationStorage.Write,
		spanRotation:      p.SpanRotation,
		serviceRotation:   p.ServiceRotation,
		tagKeysAsFields:   tags,
		allTagsAsFields:   p.AllTagsAsFields,
		tagDotReplacement: p.TagDotReplacement,
	}
}

// WriteSpan writes a span and its corresponding service:operation in ElasticSearch
func (s *SpanWriter) WriteSpan(spanStartTime time.Time, span *dbmodel.Span) {
	s.writerMetrics.Attempts.Inc(1)
	s.convertNestedTagsToFieldTags(span)
	spanIndexName := s.spanRotation.WriteTarget(spanStartTime)
	serviceIndexName := s.serviceRotation.WriteTarget(spanStartTime)
	if serviceIndexName != "" {
		s.writeService(serviceIndexName, span)
	}
	s.writeSpanToIndex(spanIndexName, span)
	s.logger.Debug("Wrote span to ES index", zap.String("index", spanIndexName))
}

func (s *SpanWriter) convertNestedTagsToFieldTags(span *dbmodel.Span) {
	// Delegates to shared assembly package. Will be removed in a future PR.
	assembly.ConvertNestedTagsToFieldTags(span, s.allTagsAsFields, s.tagKeysAsFields, s.tagDotReplacement)
}

// Close closes SpanWriter
func (s *SpanWriter) Close() error {
	return s.client().Close()
}

func keyInCache(key string, c cache.Cache) bool {
	// Delegates to shared assembly package. Will be removed in a future PR.
	return assembly.KeyInCache(key, c)
}

func writeCache(key string, c cache.Cache) {
	// Delegates to shared assembly package. Will be removed in a future PR.
	assembly.WriteCache(key, c)
}

func (s *SpanWriter) writeService(indexName string, jsonSpan *dbmodel.Span) {
	s.serviceWriter(indexName, jsonSpan)
}

func (s *SpanWriter) writeSpanToIndex(indexName string, jsonSpan *dbmodel.Span) {
	s.client().Index().Index(indexName).Type(spanType).BodyJson(&jsonSpan).Add()
}
