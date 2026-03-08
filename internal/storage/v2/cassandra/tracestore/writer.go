// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/cassandra"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

var _ tracestore.Writer = &TraceWriter{}

type TraceWriter struct {
	writer spanstore.CoreSpanWriter
}

func NewTraceWriter(
	session cassandra.Session,
	writeCacheTTL time.Duration,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
	options ...spanstore.Option,
) (*TraceWriter, error) {
	writer, err := spanstore.NewSpanWriter(session, writeCacheTTL, metricsFactory, logger, options...)
	if err != nil {
		return nil, err
	}
	return &TraceWriter{writer: writer}, nil
}

func (t *TraceWriter) WriteTraces(_ context.Context, td ptrace.Traces) error {
	dbSpans := ToDBModel(td)
	for i := range dbSpans {
		if err := t.writer.WriteSpan(&dbSpans[i]); err != nil {
			return err
		}
	}
	return nil
}

func (t *TraceWriter) Close() error {
	return t.writer.Close()
}
