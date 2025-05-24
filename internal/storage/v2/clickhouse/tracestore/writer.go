// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client"
)

type TraceWriter struct {
	conn client.Conn
}

// NewTraceWriter creates a TraceWriter instance, using connection poo to write traces to ClickHouse.
func NewTraceWriter(conn client.Conn) *TraceWriter {
	return &TraceWriter{
		conn: conn,
	}
}

func (TraceWriter) WriteTraces(context.Context, ptrace.Traces) error {
	return nil
}
