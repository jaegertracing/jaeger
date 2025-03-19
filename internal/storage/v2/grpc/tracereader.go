// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"fmt"
	"iter"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/internal/jpcommon"
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

func (*TraceReader) GetTraces(
	context.Context,
	...tracestore.GetTraceParams,
) iter.Seq2[[]ptrace.Traces, error] {
	panic("not implemented")
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

func (tr *TraceReader) FindTraceIDs(
	ctx context.Context,
	params tracestore.TraceQueryParams,
) iter.Seq2[[]tracestore.FoundTraceID, error] {
	return func(yield func([]tracestore.FoundTraceID, error) bool) {
		resp, err := tr.client.FindTraceIDs(ctx, &storage.FindTracesRequest{
			Query: toProtoQueryParameters(params),
		})
		if err != nil {
			yield(nil, fmt.Errorf("failed to execute FindTraceIDs: %w", err))
			return
		}
		foundTraceIDs := make([]tracestore.FoundTraceID, len(resp.TraceIds))
		for i, foundTraceID := range resp.TraceIds {
			var sizedTraceID [16]byte
			copy(sizedTraceID[:], foundTraceID.TraceId)

			foundTraceIDs[i] = tracestore.FoundTraceID{
				TraceID: pcommon.TraceID(sizedTraceID),
				Start:   foundTraceID.Start,
				End:     foundTraceID.End,
			}
		}
		yield(foundTraceIDs, nil)
	}
}

func toProtoQueryParameters(t tracestore.TraceQueryParams) *storage.TraceQueryParameters {
	return &storage.TraceQueryParameters{
		ServiceName:   t.ServiceName,
		OperationName: t.OperationName,
		Attributes:    jpcommon.ConvertMapToKeyValueList(t.Attributes),
		StartTimeMin:  t.StartTimeMin,
		StartTimeMax:  t.StartTimeMax,
		DurationMin:   t.DurationMin,
		DurationMax:   t.DurationMax,
		SearchDepth:   int32(t.SearchDepth), //nolint: gosec // G115
	}
}
