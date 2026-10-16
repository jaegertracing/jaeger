// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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
	if err != nil {
		wrapped := fmt.Errorf("failed to export traces: %w", err)
		// InvalidArgument rejects the batch itself, so a retry cannot succeed. Other
		// codes stay retryable, Unknown included, because remote-storage reports its
		// backend's write errors, transient ones too, as Unknown.
		if status.Code(err) == codes.InvalidArgument {
			return consumererror.NewPermanent(wrapped)
		}
		return wrapped
	}
	return nil
}
