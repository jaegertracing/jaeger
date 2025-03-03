// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package trace

import (
	"context"
	"errors"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client"
)

const INSERT_SQL = `INSERT INTO otel_traces (
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

type Writer struct {
	Client client.Pool
	logger *zap.Logger
}

func NewTraceWriter(p client.Pool, logger *zap.Logger) (*Writer, error) {
	if p == nil {
		return nil, errors.New("can't create trace writer with nil chPool")
	}
	return &Writer{Client: p, logger: logger}, nil
}

func (t *Writer) WriteTraces(ctx context.Context, td ptrace.Traces) error {
	err := t.writeTraces(ctx, td)
	if err != nil {
		return err
	}
	return nil
}

func (t *Writer) writeTraces(ctx context.Context, td ptrace.Traces) error {
	err := t.Client.Do(ctx, INSERT_SQL, td)
	if err != nil {
		return err
	}
	return nil
}
