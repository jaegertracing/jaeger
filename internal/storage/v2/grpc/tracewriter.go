// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

var _ tracestore.Writer = (*TraceWriter)(nil)

type TraceWriter struct {
	client ptraceotlp.GRPCClient
}

// NewTraceWriter creates a TraceWriter that exports traces to a remote gRPC storage server.
//
// The provided gRPC connection is used exclusively for sending trace data to the backend.
// To prevent recursive trace generation, this connection should not have instrumentation enabled.
func NewTraceWriter(conn *grpc.ClientConn) *TraceWriter {
	return &TraceWriter{
		client: ptraceotlp.NewGRPCClient(conn),
	}
}

func (tw *TraceWriter) WriteTraces(ctx context.Context, td ptrace.Traces) error {
	req := ptraceotlp.NewExportRequestFromTraces(td)
	_, err := tw.client.Export(ctx, req)
	return err
}
