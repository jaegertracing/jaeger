// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"math"
	"sync/atomic"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/proto-gen/storage/v2"
	expressionproto "github.com/jaegertracing/jaeger/internal/proto/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

var _ tracestore.Reader = (*TraceReader)(nil)

type TraceReader struct {
	client       storage.TraceReaderClient
	capabilities storage.CapabilitiesClient

	// cachedCaps holds the backend's answer once it has given one; the proto says capabilities
	// are stable for the lifetime of a connection. Failures are not cached, so a backend that
	// was not up yet is not mistaken for one that has answered. Racing callers may each ask,
	// which is harmless because they store the same answer.
	cachedCaps atomic.Pointer[tracestore.SearchCapabilities]
}

// NewTraceReader creates a TraceReader that communicates with a remote gRPC storage server.
// The provided gRPC connection is used exclusively for reading traces, meaning it is safe
// to enable instrumentation on the connection without risk of recursively generating traces.
func NewTraceReader(conn *grpc.ClientConn) *TraceReader {
	return &TraceReader{
		client:       storage.NewTraceReaderClient(conn),
		capabilities: storage.NewCapabilitiesClient(conn),
	}
}

// SearchCapabilities asks the remote backend what it supports and remembers the answer. A
// backend that does not serve the Capabilities service answers UNIMPLEMENTED, which becomes
// ErrUnsupported, so the caller assumes the least capable backend and a backend predating the
// service keeps working (RFC 0013 §3.6).
func (tr *TraceReader) SearchCapabilities(ctx context.Context) (tracestore.SearchCapabilities, error) {
	if caps := tr.cachedCaps.Load(); caps != nil {
		return *caps, nil
	}

	resp, err := tr.capabilities.GetCapabilities(ctx, &storage.GetCapabilitiesRequest{})
	if err != nil {
		if status.Code(err) == codes.Unimplemented {
			return tracestore.SearchCapabilities{}, fmt.Errorf(
				"remote storage does not report its capabilities: %w", errors.ErrUnsupported,
			)
		}
		return tracestore.SearchCapabilities{}, err
	}
	caps := tracestore.SearchCapabilities{
		WithoutServiceName:  resp.GetSearch().GetWithoutServiceName(),
		SameSpanConjunction: resp.GetSearch().GetSameSpanConjunction(),
		Filter:              fromProtoFilterCapabilities(resp.GetSearch().GetFilter()),
	}
	tr.cachedCaps.Store(&caps)
	return caps, nil
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
		query, err := toProtoQueryParameters(params)
		if err != nil {
			yield(nil, err)
			return
		}
		stream, err := tr.client.FindTraces(ctx, &storage.FindTracesRequest{Query: query})
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
		query, err := toProtoQueryParameters(params)
		if err != nil {
			yield(nil, err)
			return
		}
		resp, err := tr.client.FindTraceIDs(ctx, &storage.FindTraceIDsRequest{Query: query})
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

func (tr *TraceReader) FindTraceSummaries(
	ctx context.Context,
	params tracestore.TraceQueryParams,
) iter.Seq2[[]tracestore.TraceSummary, error] {
	maybeNotImplemented := func(err error, msg string) error {
		if status.Code(err) == codes.Unimplemented || errors.Is(err, errors.ErrUnsupported) {
			return fmt.Errorf("remote server does not support FindTraceSummaries: %w", errors.ErrUnsupported)
		}
		return fmt.Errorf("%s: %w", msg, err)
	}
	return func(yield func([]tracestore.TraceSummary, error) bool) {
		query, err := toProtoQueryParameters(params)
		if err != nil {
			yield(nil, err)
			return
		}
		stream, err := tr.client.FindTraceSummaries(ctx, &storage.FindTraceSummariesRequest{Query: query})
		if err != nil {
			yield(nil, maybeNotImplemented(err, "failed to execute FindTraceSummaries"))
			return
		}
		for {
			resp, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				yield(nil, maybeNotImplemented(err, "received error from grpc stream"))
				return
			}
			if !yield(convertSummaryBatch(resp.GetSummaries()), nil) {
				return
			}
		}
	}
}

func convertSummaryBatch(protos []*storage.TraceSummary) []tracestore.TraceSummary {
	batch := make([]tracestore.TraceSummary, len(protos))
	for i, ps := range protos {
		var traceID [16]byte
		copy(traceID[:], ps.GetTraceId())
		svcs := make([]tracestore.ServiceSummary, len(ps.GetServices()))
		for j, ss := range ps.GetServices() {
			svcs[j] = tracestore.ServiceSummary{
				Name:           ss.GetName(),
				SpanCount:      int(ss.GetSpanCount()),
				ErrorSpanCount: int(ss.GetErrorSpanCount()),
			}
		}
		batch[i] = tracestore.TraceSummary{
			TraceID:           pcommon.TraceID(traceID),
			RootServiceName:   ps.GetRootServiceName(),
			RootOperationName: ps.GetRootOperationName(),
			MinStartTime:      jptrace.UnixNanoToTime(ps.GetMinStartTimeUnixNano()),
			MaxEndTime:        jptrace.UnixNanoToTime(ps.GetMaxEndTimeUnixNano()),
			SpanCount:         int(ps.GetSpanCount()),
			ErrorSpanCount:    int(ps.GetErrorSpanCount()),
			OrphanSpanCount:   int(ps.GetOrphanSpanCount()),
			Services:          svcs,
		}
	}
	return batch
}

// toProtoQueryParameters encodes a query for the remote server. Encoding the filter can fail, for
// a tree the wire has no form for, and asking here rather than at each RPC keeps that one refusal in
// one place: the alternative is sending a query whose filter went missing, which reads to the server
// as a search with no predicates.
func toProtoQueryParameters(t tracestore.TraceQueryParams) (*storage.TraceQueryParameters, error) {
	filter, err := expressionproto.ToProto(t.Filter)
	if err != nil {
		return nil, fmt.Errorf("cannot send the query filter: %w", err)
	}
	if t.SearchDepth < math.MinInt32 || t.SearchDepth > math.MaxInt32 {
		return nil, fmt.Errorf("SearchDepth must be in [%d, %d]", math.MinInt32, math.MaxInt32)
	}
	return &storage.TraceQueryParameters{
		ServiceName:   t.ServiceName,
		OperationName: t.OperationName,
		Attributes:    convertMapToKeyValueList(t.Attributes),
		StartTimeMin:  t.StartTimeMin,
		StartTimeMax:  t.StartTimeMax,
		DurationMin:   t.DurationMin,
		DurationMax:   t.DurationMax,
		SearchDepth:   int32(t.SearchDepth),
		Filter:        filter,
	}, nil
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
