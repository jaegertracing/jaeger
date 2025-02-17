// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/clickhouse"
)

var insertTrace = `INSERT INTO %s (
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
	Client clickhouse.Client
	table  string
	logger *zap.Logger
}

func NewTraceWriter(client clickhouse.Client, logger *zap.Logger, table string) (*TraceWriter, error) {
	return &TraceWriter{Client: client, logger: logger, table: table}, nil
}

func (t *TraceWriter) WriteTraces(ctx context.Context, td ptrace.Traces) error {
	err := t.writeTraces(ctx, td)
	if err != nil {
		return err
	}
	return nil
}

func (t *TraceWriter) writeTraces(ctx context.Context, td ptrace.Traces) error {
	// TODO SQL injection?
	param := clickhouse.ChQuery{
		Body:  fmt.Sprintf(insertTrace, t.table),
		Input: td,
	}
	err := t.Client.Do(ctx, param)
	if err != nil {
		return err
	}
	return nil
}
