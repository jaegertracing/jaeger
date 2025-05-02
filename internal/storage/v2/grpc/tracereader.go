// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/internal/proto-gen/storage/v2"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
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
			yield(nil, fmt.Errorf("failed to execute GetTraces: %w", err))
			return
		}
		for received, err := stream.Recv(); !errors.Is(err, io.EOF); received, err = stream.Recv() {
			if err != nil {
				yield(nil, fmt.Errorf("received error from grpc stream: %w", err))
				return
			}
			if !yield([]ptrace.Traces{received.ToTraces()}, nil) {
				return
			}
		}
	}
}

func (tr *TraceReader) GetServices(ctx context.Context) ([]string, error) {
	resp, err := tr.client.GetServices(ctx, &storage.GetServicesRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to execute GetServices: %w", err)
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
		return nil, fmt.Errorf("failed to execute GetOperations: %w", err)
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

func (tr *TraceReader) FindTraces(
	ctx context.Context,
	params tracestore.TraceQueryParams,
) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		stream, err := tr.client.FindTraces(ctx, &storage.FindTracesRequest{
			Query: toProtoQueryParameters(params),
		})
		if err != nil {
			yield(nil, fmt.Errorf("failed to execute FindTraces: %w", err))
			return
		}
		for received, err := stream.Recv(); !errors.Is(err, io.EOF); received, err = stream.Recv() {
			if err != nil {
				yield(nil, fmt.Errorf("received error from grpc stream: %w", err))
				return
			}
			if !yield([]ptrace.Traces{received.ToTraces()}, nil) {
				return
			}
		}
	}
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
		Attributes:    convertMapToKeyValueList(t.Attributes),
		StartTimeMin:  t.StartTimeMin,
		StartTimeMax:  t.StartTimeMax,
		DurationMin:   t.DurationMin,
		DurationMax:   t.DurationMax,
		SearchDepth:   int32(t.SearchDepth), //nolint: gosec // G115
	}
}

func convertMapToKeyValueList(m pcommon.Map) []*storage.KeyValue {
	keyValues := make([]*storage.KeyValue, 0, m.Len())
	m.Range(func(k string, v pcommon.Value) bool {
		keyValues = append(keyValues, &storage.KeyValue{
			Key:   k,
			Value: convertValueToAnyValue(v),
		})
		return true
	})
	return keyValues
}

func convertValueToAnyValue(v pcommon.Value) *storage.AnyValue {
	switch v.Type() {
	case pcommon.ValueTypeStr:
		return &storage.AnyValue{
			Value: &storage.AnyValue_StringValue{
				StringValue: v.Str(),
			},
		}
	case pcommon.ValueTypeBool:
		return &storage.AnyValue{
			Value: &storage.AnyValue_BoolValue{
				BoolValue: v.Bool(),
			},
		}
	case pcommon.ValueTypeInt:
		return &storage.AnyValue{
			Value: &storage.AnyValue_IntValue{
				IntValue: v.Int(),
			},
		}
	case pcommon.ValueTypeDouble:
		return &storage.AnyValue{
			Value: &storage.AnyValue_DoubleValue{
				DoubleValue: v.Double(),
			},
		}
	case pcommon.ValueTypeBytes:
		return &storage.AnyValue{
			Value: &storage.AnyValue_BytesValue{
				BytesValue: v.Bytes().AsRaw(),
			},
		}
	case pcommon.ValueTypeSlice:
		arr := v.Slice()
		arrayValues := make([]*storage.AnyValue, 0, arr.Len())
		for i := 0; i < arr.Len(); i++ {
			arrayValues = append(arrayValues, convertValueToAnyValue(arr.At(i)))
		}
		return &storage.AnyValue{
			Value: &storage.AnyValue_ArrayValue{
				ArrayValue: &storage.ArrayValue{
					Values: arrayValues,
				},
			},
		}
	case pcommon.ValueTypeMap:
		kvList := &storage.KeyValueList{}
		v.Map().Range(func(k string, val pcommon.Value) bool {
			kvList.Values = append(kvList.Values, &storage.KeyValue{
				Key:   k,
				Value: convertValueToAnyValue(val),
			})
			return true
		})
		return &storage.AnyValue{Value: &storage.AnyValue_KvlistValue{KvlistValue: kvList}}
	default:
		return nil
	}
}
