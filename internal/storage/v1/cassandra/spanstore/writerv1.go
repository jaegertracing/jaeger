// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/cassandra"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore/dbmodel"
)

var _ spanstore.Writer = &SpanWriterV1{}

type SpanWriterV1 struct {
	spanWriter CoreSpanWriter
	tagFilter  dbmodel.TagFilter
}

func NewSpanWriterV1(
	session cassandra.Session,
	writeCacheTTL time.Duration,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
	options ...Option,
) (*SpanWriterV1, error) {
	opts := applyOptions(options...)
	writer, err := NewSpanWriter(session, writeCacheTTL, metricsFactory, logger, opts)
	if err != nil {
		return nil, err
	}
	return &SpanWriterV1{spanWriter: writer, tagFilter: opts.tagFilter}, nil
}

func (s *SpanWriterV1) WriteSpan(_ context.Context, span *model.Span) error {
	ds := dbmodel.FromDomain(span)
	insertions := dbmodel.GetAllUniqueTags(span, s.tagFilter)
	return s.spanWriter.WriteSpan(ds, insertions)
}

func (s *SpanWriterV1) Close() error {
	return s.spanWriter.Close()
}
