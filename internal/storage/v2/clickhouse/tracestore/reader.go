// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"iter"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client"
)

type TraceReader struct {
	conn client.Conn
}

// NewTraceReader creates a TraceReader instance, using the connection to get traces from ClickHouse.
func NewTraceReader(conn client.Conn) *TraceReader {
	return &TraceReader{
		conn: conn,
	}
}

func (TraceReader) GetTraces(context.Context, ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
	return nil
}

func (TraceReader) GetServices(context.Context) ([]string, error) {
	return nil, nil
}

func (TraceReader) GetOperations(context.Context, tracestore.OperationQueryParams) ([]tracestore.Operation, error) {
	return nil, nil
}

func (TraceReader) FindTraces(context.Context, tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
	return nil
}

func (TraceReader) FindTraceIDs(context.Context, tracestore.TraceQueryParams) iter.Seq2[[]tracestore.FoundTraceID, error] {
	return nil
}
