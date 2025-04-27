// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	cfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

type SpanWriterV1 struct {
	spanWriter CoreSpanWriter
}

// NewSpanWriterV1 returns the SpanWriterV1 for use
func NewSpanWriterV1(p SpanWriterParams) *SpanWriterV1 {
	return &SpanWriterV1{
		spanWriter: NewSpanWriter(p),
	}
}

// CreateTemplates creates index templates.
func (s *SpanWriterV1) CreateTemplates(spanTemplate, serviceTemplate string, indexPrefix cfg.IndexPrefix) error {
	return s.spanWriter.CreateTemplates(spanTemplate, serviceTemplate, indexPrefix)
}

// WriteSpan writes a span and its corresponding service:operation in ElasticSearch
func (s *SpanWriterV1) WriteSpan(_ context.Context, span *model.Span) error {
	converter := NewFromDomain()
	jsonSpan := converter.FromDomainEmbedProcess(span)
	jsonSpan := s.spanConverter.FromDomainEmbedProcess(span)
	return s.spanWriter.WriteSpan(span.StartTime, jsonSpan)
}

// Close closes SpanWriter
func (s *SpanWriterV1) Close() error {
	return s.spanWriter.Close()
}
