// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/proto-gen/storage/v2"
)

var _ tracestore.Reader = (*TraceReader)(nil)

var errFailedToGetServices = errors.New("failed to get services")

type TraceReader struct {
	client storage.TraceReaderClient
}

// NewTraceReader creates a TraceReader that communicates with a remote gRPC storage server.
// The provided gRPC connection is used exclusively for reading traces, meaning it is safe
// to enable instrumentation on the connection without risk of recursively generating traces.
func NewTraceReader(conn *grpc.ClientConn) *TraceReader {
	return &TraceReader{
		client: storage.NewTraceReaderClient(conn),
	}
}

func (*TraceReader) GetTraces(
	context.Context,
	...tracestore.GetTraceParams,
) iter.Seq2[[]ptrace.Traces, error] {
	panic("not implemented")
}

func (tr *TraceReader) GetServices(ctx context.Context) ([]string, error) {
	resp, err := tr.client.GetServices(ctx, &storage.GetServicesRequest{})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errFailedToGetServices, err)
	}
	return resp.Services, nil
}

func (*TraceReader) GetOperations(
	context.Context,
	tracestore.OperationQueryParams,
) ([]tracestore.Operation, error) {
	panic("not implemented")
}

func (*TraceReader) FindTraces(
	context.Context,
	tracestore.TraceQueryParams,
) iter.Seq2[[]ptrace.Traces, error] {
	panic("not implemented")
}

func (*TraceReader) FindTraceIDs(
	context.Context,
	tracestore.TraceQueryParams,
) iter.Seq2[[]tracestore.FoundTraceID, error] {
	panic("not implemented")
}
