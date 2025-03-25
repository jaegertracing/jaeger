// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client"
)

const writeTraces = `INSERT INTO otel_traces (
	Timestamp,
	TraceId,
	SpanId,
	ParentSpanId,
	TraceState,
	SpanName,
	SpanKind,
	ServiceName,
	ResourceAttributes.keys,
	ResourceAttributes.values,
	ScopeName,
	ScopeVersion,
	SpanAttributes.keys,
	SpanAttributes.values,
	Duration,
	StatusCode,
	StatusMessage,
	Events.Timestamp,
	Events.Name,
    Events.Attributes,
    Links.TraceId,
    Links.SpanId,
	Links.TraceState,
    Links.Attributes
	) VALUES`

type TraceWriter struct {
	Client client.ChPool
	logger *zap.Logger
}

func NewTraceWriter(p client.ChPool, logger *zap.Logger) (*TraceWriter, error) {
	if p == nil {
		return nil, errors.New("can't create trace writer with nil chPool")
	}
	return &TraceWriter{Client: p, logger: logger}, nil
}

func (t *TraceWriter) WriteTraces(ctx context.Context, td ptrace.Traces) error {
	err := t.writeTraces(ctx, td)
	if err != nil {
		return err
	}
	return nil
}

func (t *TraceWriter) writeTraces(ctx context.Context, td ptrace.Traces) error {
	err := t.Client.Do(ctx, writeTraces, td)
	if err != nil {
		return err
	}
	return nil
}
