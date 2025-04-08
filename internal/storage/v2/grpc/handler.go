// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/proto-gen/storage/v2"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

var (
	_ storage.TraceReaderServer      = (*Handler)(nil)
	_ storage.DependencyReaderServer = (*Handler)(nil)
	_ ptraceotlp.GRPCServer          = (*Handler)(nil)
)

type Handler struct {
	storage.UnimplementedTraceReaderServer
	storage.UnimplementedDependencyReaderServer
	ptraceotlp.UnimplementedGRPCServer

	traceReader tracestore.Reader
}

func NewHandler(traceReader tracestore.Reader) *Handler {
	return &Handler{
		traceReader: traceReader,
	}
}

func (h *Handler) GetTraces(
	req *storage.GetTracesRequest,
	srv storage.TraceReader_GetTracesServer,
) error {
	traceIDs := make([]tracestore.GetTraceParams, len(req.Query))
	for i, query := range req.Query {
		var sizedTraceID [16]byte
		copy(sizedTraceID[:], query.TraceId)

		traceIDs[i] = tracestore.GetTraceParams{
			TraceID: pcommon.TraceID(sizedTraceID),
			Start:   query.StartTime,
			End:     query.EndTime,
		}
	}
	for traces, err := range h.traceReader.GetTraces(srv.Context(), traceIDs...) {
		if err != nil {
			return err
		}
		for _, trace := range traces {
			td := jptrace.TracesData(trace)
			if err = srv.Send(&td); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *Handler) GetServices(
	ctx context.Context,
	_ *storage.GetServicesRequest,
) (*storage.GetServicesResponse, error) {
	services, err := h.traceReader.GetServices(ctx)
	if err != nil {
		return nil, err
	}
	return &storage.GetServicesResponse{
		Services: services,
	}, nil
}

func (h *Handler) GetOperations(
	ctx context.Context,
	req *storage.GetOperationsRequest,
) (*storage.GetOperationsResponse, error) {
	operations, err := h.traceReader.GetOperations(ctx, tracestore.OperationQueryParams{
		ServiceName: req.Service,
		SpanKind:    req.SpanKind,
	})
	if err != nil {
		return nil, err
	}
	grpcOperations := make([]*storage.Operation, len(operations))
	for i, operation := range operations {
		grpcOperations[i] = &storage.Operation{
			Name:     operation.Name,
			SpanKind: operation.SpanKind,
		}
	}
	return &storage.GetOperationsResponse{
		Operations: grpcOperations,
	}, nil
}

func (h *Handler) FindTraces(
	req *storage.FindTracesRequest,
	srv storage.TraceReader_FindTracesServer,
) error {
	query := tracestore.TraceQueryParams{
		ServiceName:   req.Query.ServiceName,
		OperationName: req.Query.OperationName,
		Attributes:    convertKeyValueListToMap(req.Query.Attributes),
		StartTimeMin:  req.Query.StartTimeMin,
		StartTimeMax:  req.Query.StartTimeMax,
		DurationMin:   req.Query.DurationMin,
		DurationMax:   req.Query.DurationMax,
		SearchDepth:   int(req.Query.SearchDepth),
	}
	for traces, err := range h.traceReader.FindTraces(srv.Context(), query) {
		if err != nil {
			return err
		}
		for _, trace := range traces {
			td := jptrace.TracesData(trace)
			if err = srv.Send(&td); err != nil {
				return err
			}
		}
	}
	return nil
}

func convertKeyValueListToMap(kvList []*storage.KeyValue) pcommon.Map {
	m := pcommon.NewMap()
	for _, kv := range kvList {
		if kv == nil || kv.Value == nil {
			continue
		}
		setValueToMap(m, kv.Key, kv.Value)
	}
	return m
}

func setValueToMap(m pcommon.Map, key string, av *storage.AnyValue) {
	switch v := av.Value.(type) {
	case *storage.AnyValue_StringValue:
		m.PutStr(key, v.StringValue)
	case *storage.AnyValue_BoolValue:
		m.PutBool(key, v.BoolValue)
	case *storage.AnyValue_IntValue:
		m.PutInt(key, v.IntValue)
	case *storage.AnyValue_DoubleValue:
		m.PutDouble(key, v.DoubleValue)
	case *storage.AnyValue_BytesValue:
		m.PutEmptyBytes(key).FromRaw(v.BytesValue)
	case *storage.AnyValue_ArrayValue:
		sliceVal := m.PutEmptySlice(key)
		for _, elem := range v.ArrayValue.Values {
			if elem == nil {
				sliceVal.AppendEmpty()
				continue
			}
			setValueToSlice(sliceVal, elem)
		}
	case *storage.AnyValue_KvlistValue:
		mapVal := m.PutEmptyMap(key)
		for _, kv := range v.KvlistValue.Values {
			if kv == nil || kv.Value == nil {
				continue
			}
			setValueToMap(mapVal, kv.Key, kv.Value)
		}
	}
}

func setValueToSlice(slice pcommon.Slice, av *storage.AnyValue) {
	switch v := av.Value.(type) {
	case *storage.AnyValue_StringValue:
		slice.AppendEmpty().SetStr(v.StringValue)
	case *storage.AnyValue_BoolValue:
		slice.AppendEmpty().SetBool(v.BoolValue)
	case *storage.AnyValue_IntValue:
		slice.AppendEmpty().SetInt(v.IntValue)
	case *storage.AnyValue_DoubleValue:
		slice.AppendEmpty().SetDouble(v.DoubleValue)
	case *storage.AnyValue_BytesValue:
		slice.AppendEmpty().SetEmptyBytes().FromRaw(v.BytesValue)
	case *storage.AnyValue_ArrayValue:
		newSlice := slice.AppendEmpty().SetEmptySlice()
		for _, subElem := range v.ArrayValue.Values {
			if subElem == nil {
				newSlice.AppendEmpty()
				continue
			}
			setValueToSlice(newSlice, subElem)
		}
	case *storage.AnyValue_KvlistValue:
		newMap := slice.AppendEmpty().SetEmptyMap()
		for _, kv := range v.KvlistValue.Values {
			if kv == nil || kv.Value == nil {
				continue
			}
			setValueToMap(newMap, kv.Key, kv.Value)
		}
	}
}
