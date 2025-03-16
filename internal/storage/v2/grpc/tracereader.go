// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/proto-gen/storage/v2"
)

var _ tracestore.Reader = (*TraceReader)(nil)

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

func (tr *TraceReader) GetTraces(
	ctx context.Context,
	traceIDs ...tracestore.GetTraceParams,
) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		query := []*storage.GetTraceParams{}
		for _, traceID := range traceIDs {
			query = append(query, &storage.GetTraceParams{
				TraceId:   traceID.TraceID[:],
				StartTime: traceID.Start,
				EndTime:   traceID.End,
			})
		}
		stream, err := tr.client.GetTraces(ctx, &storage.GetTracesRequest{
			Query: query,
		})
		if err != nil {
			yield(nil, fmt.Errorf("received error from grpc reader client: %w", err))
			return
		}
		for received, err := stream.Recv(); !errors.Is(err, io.EOF); received, err = stream.Recv() {
			if err != nil {
				yield(nil, fmt.Errorf("received error from grpc stream: %w", err))
				return
			}
			receivedTraces := make([]ptrace.Traces, len(received.Traces))
			for i, traces := range received.Traces {
				receivedTraces[i] = traces.ToTraces()
			}
			if !yield(receivedTraces, nil) {
				return
			}
		}
	}
}

func (tr *TraceReader) GetServices(ctx context.Context) ([]string, error) {
	resp, err := tr.client.GetServices(ctx, &storage.GetServicesRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get services: %w", err)
	}
	return resp.Services, nil
}

func (tr *TraceReader) GetOperations(
	ctx context.Context,
	params tracestore.OperationQueryParams,
) ([]tracestore.Operation, error) {
	resp, err := tr.client.GetOperations(ctx, &storage.GetOperationsRequest{
		Service:  params.ServiceName,
		SpanKind: params.SpanKind,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get operations: %w", err)
	}
	operations := make([]tracestore.Operation, len(resp.Operations))
	for i, op := range resp.Operations {
		operations[i] = tracestore.Operation{
			Name:     op.Name,
			SpanKind: op.SpanKind,
		}
	}
	return operations, nil
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
