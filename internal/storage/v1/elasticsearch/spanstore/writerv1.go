// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/spanstoremetrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore/internal/dbmodel"
	cfg "github.com/jaegertracing/jaeger/pkg/es/config"
)

type SpanWriterV1 struct {
	spanWriter    CoreSpanWriter
	spanConverter dbmodel.FromDomain
	writerMetrics spanWriterMetrics // TODO: build functions to wrap around each Do fn
}

type spanWriterMetrics struct {
	indexCreate *spanstoremetrics.WriteMetrics
}

// NewSpanWriterV1 returns the SpanWriterV1 for use
func NewSpanWriterV1(p SpanWriterParams) *SpanWriterV1 {
	return &SpanWriterV1{
		spanWriter:    NewSpanWriter(p),
		spanConverter: dbmodel.NewFromDomain(p.AllTagsAsFields, p.TagKeysAsFields, p.TagDotReplacement),
		writerMetrics: spanWriterMetrics{
			indexCreate: spanstoremetrics.NewWriter(p.MetricsFactory, "index_create"),
		},
	}
}

// CreateTemplates creates index templates.
func (s *SpanWriterV1) CreateTemplates(spanTemplate, serviceTemplate string, indexPrefix cfg.IndexPrefix) error {
	return s.spanWriter.CreateTemplates(spanTemplate, serviceTemplate, indexPrefix)
}

// WriteSpan writes a span and its corresponding service:operation in ElasticSearch
func (s *SpanWriterV1) WriteSpan(_ context.Context, span *model.Span) error {
	jsonSpan := s.spanConverter.FromDomainEmbedProcess(span)
	s.spanWriter.WriteSpan(span.StartTime, jsonSpan)
	return nil
}

// Close closes SpanWriter
func (s *SpanWriterV1) Close() error {
	return s.spanWriter.Close()
}
