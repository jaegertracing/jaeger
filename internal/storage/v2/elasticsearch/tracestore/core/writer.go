// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/cache"
	"github.com/jaegertracing/jaeger/internal/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	cfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/indices"
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
	SpanWriteAlias      string
	ServiceWriteAlias   string
	ServiceCacheTTL     time.Duration
	SpanRotation        indices.Rotation
	ServiceRotation     indices.Rotation
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

	spanRotation, serviceRotation := buildWriterRotations(p)

	serviceOperationStorage := NewServiceOperationStorage(p.Client, p.Logger, serviceCacheTTL)
	return &SpanWriter{
		client:            p.Client,
		logger:            p.Logger,
		writerMetrics:     spanstoremetrics.NewWriter(p.MetricsFactory, "spans"),
		serviceWriter:     serviceOperationStorage.Write,
		spanRotation:      spanRotation,
		serviceRotation:   serviceRotation,
		tagKeysAsFields:   tags,
		allTagsAsFields:   p.AllTagsAsFields,
		tagDotReplacement: p.TagDotReplacement,
	}
}

func buildWriterRotations(p SpanWriterParams) (spanRotation, serviceRotation indices.Rotation) {
	if p.SpanRotation != nil {
		spanRotation = p.SpanRotation
	}
	if p.ServiceRotation != nil {
		serviceRotation = p.ServiceRotation
	}
	if spanRotation != nil && serviceRotation != nil {
		return spanRotation, serviceRotation
	}

	writeAliasSuffix := ""
	if p.UseReadWriteAliases {
		if p.WriteAliasSuffix != "" {
			writeAliasSuffix = p.WriteAliasSuffix
		} else {
			writeAliasSuffix = "write"
		}
	}

	spanAndServiceFn := getSpanAndServiceIndexFn(p, writeAliasSuffix)
	if spanRotation == nil {
		spanRotation = &fnRotation{fn: func(t time.Time) string {
			span, _ := spanAndServiceFn(t)
			return span
		}}
	}
	if serviceRotation == nil {
		serviceRotation = &fnRotation{fn: func(t time.Time) string {
			_, svc := spanAndServiceFn(t)
			return svc
		}}
	}
	return spanRotation, serviceRotation
}

// spanAndServiceIndexFn returns names of span and service indices.
//
// Deprecated: use indices.Rotation instead.
type spanAndServiceIndexFn func(spanTime time.Time) (string, string)

// fnRotation adapts the legacy closure to the Rotation interface (write-only).
type fnRotation struct {
	fn func(time.Time) string
}

func (f *fnRotation) WriteTarget(t time.Time) string          { return f.fn(t) }
func (*fnRotation) ReadTargets(time.Time, time.Time) []string { return nil }
func (*fnRotation) WriteOpType() indices.WriteOpType           { return indices.WriteOpIndex }

func getSpanAndServiceIndexFn(p SpanWriterParams, writeAlias string) spanAndServiceIndexFn {
	// If explicit write aliases are provided, use them directly without modification
	if p.SpanWriteAlias != "" && p.ServiceWriteAlias != "" {
		return func(_ time.Time) (string, string) {
			return p.SpanWriteAlias, p.ServiceWriteAlias
		}
	}

	// Otherwise, use the standard prefix + suffix approach
	spanIndexPrefix := p.IndexPrefix.Apply(spanIndexBaseName)
	serviceIndexPrefix := p.IndexPrefix.Apply(serviceIndexBaseName)

	if p.UseReadWriteAliases {
		return func(_ time.Time) (string, string) {
			return spanIndexPrefix + writeAlias, serviceIndexPrefix + writeAlias
		}
	}

	return func(date time.Time) (string, string) {
		return indices.IndexWithDate(spanIndexPrefix, p.SpanIndex.DateLayout, date), indices.IndexWithDate(serviceIndexPrefix, p.ServiceIndex.DateLayout, date)
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

func (s *SpanWriter) writeSpanToIndex(indexName string, jsonSpan *dbmodel.Span) {
	s.client().Index().Index(indexName).Type(spanType).BodyJson(&jsonSpan).Add()
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
