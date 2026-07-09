// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/cache"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/indices"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/spanstoremetrics"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

const (
	serviceCacheTTLDefault = 12 * time.Hour
	indexCacheTTLDefault   = 48 * time.Hour
)

type serviceWriter func(string, *dbmodel.Span)

// SpanWriter writes spans and their service:operation pairs via the bulk indexer.
type SpanWriter struct {
	bulkWriter        esclient.BulkWriter
	logger            *zap.Logger
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
	// WriteSpans writes a batch of spans and their corresponding service:operation
	// pairs to Elasticsearch/OpenSearch. It is the batch entry point the v2
	// TraceWriter drives once per WriteTraces call. The async implementation
	// enqueues each document into the shared bulk indexer and returns nil (an
	// enqueue cannot fail synchronously); a synchronous implementation issues one
	// blocking _bulk per batch and returns the real write error (RFC 0007).
	WriteSpans(ctx context.Context, spans []dbmodel.Span) error
	// Close closes Writer
	Close() error
}

// SpanWriterParams holds constructor parameters for NewSpanWriter. BulkWriter is
// required — the writer enqueues every span and service document through it.
type SpanWriterParams struct {
	BulkWriter        esclient.BulkWriter
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

	// The writer only calls Write (never the read methods), so it needs no searcher.
	serviceOperationStorage := NewServiceOperationStorage(nil, p.BulkWriter, p.Logger, serviceCacheTTL)
	return &SpanWriter{
		bulkWriter:        p.BulkWriter,
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

// WriteSpans enqueues a batch of spans and their service:operation pairs into the
// bulk indexer. Because the indexer is asynchronous (fire-and-forget), the enqueue
// cannot fail synchronously, so this always returns nil; per-item failures surface
// in the indexer's OnFailure callback. RFC 0007 adds a synchronous peer that issues
// one blocking _bulk per batch and returns the real error.
func (s *SpanWriter) WriteSpans(_ context.Context, spans []dbmodel.Span) error {
	for i := range spans {
		span := &spans[i]
		s.writeSpan(model.EpochMicrosecondsAsTime(span.StartTime), span)
	}
	return nil
}

// writeSpan writes a single span and its corresponding service:operation into the
// bulk indexer.
func (s *SpanWriter) writeSpan(spanStartTime time.Time, span *dbmodel.Span) {
	s.writerMetrics.Attempts.Inc(1)
	s.convertNestedTagsToFieldTags(span)
	if s.spanRotation.RequiresDocumentTimestamp() {
		span.Timestamp = strconv.FormatInt(spanStartTime.UnixNano(), 10)
	}
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

// Close is a no-op: the writer owns no resources. The bulk indexer that backs it
// is owned and closed by the factory (which flushes buffered writes on shutdown).
func (*SpanWriter) Close() error {
	return nil
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
	// A SpanWriter always has a bulkWriter (a required SpanWriterParams field), so
	// unlike ServiceOperationStorage.Write — which also serves read-only instances
	// with a nil writer — this is not nil-checked.
	s.bulkWriter.Add(esclient.BulkItem{
		Index:  indexName,
		OpType: s.spanRotation.WriteOpType(),
		Body:   jsonSpan,
	})
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
