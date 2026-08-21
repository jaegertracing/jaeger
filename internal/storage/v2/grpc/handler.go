// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"errors"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/jptrace/sanitizer"
	"github.com/jaegertracing/jaeger/internal/proto-gen/storage/v2"
	expressionproto "github.com/jaegertracing/jaeger/internal/proto/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

var (
	_ storage.TraceReaderServer      = (*Handler)(nil)
	_ storage.DependencyReaderServer = (*Handler)(nil)
	_ storage.CapabilitiesServer     = (*Handler)(nil)
	_ ptraceotlp.GRPCServer          = (*Handler)(nil)
)

type Handler struct {
	storage.UnimplementedTraceReaderServer
	storage.UnimplementedDependencyReaderServer
	storage.UnimplementedCapabilitiesServer
	ptraceotlp.UnimplementedGRPCServer

	traceReader tracestore.Reader
	traceWriter tracestore.Writer
	depReader   depstore.Reader
}

func NewHandler(
	traceReader tracestore.Reader,
	traceWriter tracestore.Writer,
	depReader depstore.Reader,
) *Handler {
	return &Handler{
		traceReader: traceReader,
		traceWriter: traceWriter,
		depReader:   depReader,
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
			err = srv.Send(&td)
			if err != nil {
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
	query, err := h.toTraceQueryParams(srv.Context(), req.Query)
	if err != nil {
		return err
	}
	for traces, err := range h.traceReader.FindTraces(srv.Context(), query) {
		if err != nil {
			return err
		}
		for _, trace := range traces {
			td := jptrace.TracesData(trace)
			err = srv.Send(&td)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *Handler) FindTraceSummaries(
	req *storage.FindTraceSummariesRequest,
	srv storage.TraceReader_FindTraceSummariesServer,
) error {
	query, err := h.toTraceQueryParams(srv.Context(), req.Query)
	if err != nil {
		return err
	}
	for summaries, err := range h.traceReader.FindTraceSummaries(srv.Context(), query) {
		if err != nil {
			// A backend that cannot compute summaries natively signals this with
			// errors.ErrUnsupported; surface it as gRPC Unimplemented so the remote
			// client falls back to loading full traces and aggregating client-side.
			if errors.Is(err, errors.ErrUnsupported) {
				return status.Errorf(codes.Unimplemented, "method FindTraceSummaries not implemented: %v", err)
			}
			return err
		}
		batch := make([]*storage.TraceSummary, len(summaries))
		for i := range summaries {
			s := &summaries[i]
			svcs := make([]*storage.ServiceSummary, len(s.Services))
			for j := range s.Services {
				svcs[j] = &storage.ServiceSummary{
					Name:           s.Services[j].Name,
					SpanCount:      int32(s.Services[j].SpanCount),      //nolint:gosec // G115
					ErrorSpanCount: int32(s.Services[j].ErrorSpanCount), //nolint:gosec // G115
				}
			}
			batch[i] = &storage.TraceSummary{
				TraceId:              s.TraceID[:],
				RootServiceName:      s.RootServiceName,
				RootOperationName:    s.RootOperationName,
				MinStartTimeUnixNano: jptrace.TimeToUnixNano(s.MinStartTime),
				MaxEndTimeUnixNano:   jptrace.TimeToUnixNano(s.MaxEndTime),
				SpanCount:            int32(s.SpanCount),       //nolint:gosec // G115
				ErrorSpanCount:       int32(s.ErrorSpanCount),  //nolint:gosec // G115
				OrphanSpanCount:      int32(s.OrphanSpanCount), //nolint:gosec // G115
				Services:             svcs,
			}
		}
		if err := srv.Send(&storage.FindTraceSummariesResponse{Summaries: batch}); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) FindTraceIDs(
	ctx context.Context,
	req *storage.FindTraceIDsRequest,
) (*storage.FindTraceIDsResponse, error) {
	foundTraceIDs := []*storage.FoundTraceID{}
	query, err := h.toTraceQueryParams(ctx, req.Query)
	if err != nil {
		return nil, err
	}
	for traceIDs, err := range h.traceReader.FindTraceIDs(ctx, query) {
		if err != nil {
			return nil, err
		}
		for _, traceID := range traceIDs {
			foundTraceIDs = append(foundTraceIDs, &storage.FoundTraceID{
				TraceId: traceID.TraceID[:],
				Start:   traceID.Start,
				End:     traceID.End,
			})
		}
	}
	return &storage.FindTraceIDsResponse{
		TraceIds: foundTraceIDs,
	}, nil
}

func (h *Handler) Export(ctx context.Context, request ptraceotlp.ExportRequest) (
	ptraceotlp.ExportResponse,
	error,
) {
	err := h.traceWriter.WriteTraces(ctx, sanitizer.Sanitize(request.Traces()))
	if err != nil {
		return ptraceotlp.NewExportResponse(), err
	}
	return ptraceotlp.NewExportResponse(), nil
}

func (h *Handler) GetDependencies(
	ctx context.Context,
	req *storage.GetDependenciesRequest,
) (*storage.GetDependenciesResponse, error) {
	dependencies, err := h.depReader.GetDependencies(ctx, depstore.QueryParameters{
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
	})
	if err != nil {
		return nil, err
	}
	grpcDependencies := make([]*storage.Dependency, len(dependencies))
	for i, dependency := range dependencies {
		grpcDependencies[i] = &storage.Dependency{
			Parent:    dependency.Parent,
			Child:     dependency.Child,
			CallCount: dependency.CallCount,
			Source:    dependency.Source,
		}
	}
	return &storage.GetDependenciesResponse{
		Dependencies: grpcDependencies,
	}, nil
}

func (h *Handler) Register(ss *grpc.Server, hs *health.Server) {
	storage.RegisterTraceReaderServer(ss, h)
	storage.RegisterDependencyReaderServer(ss, h)
	storage.RegisterCapabilitiesServer(ss, h)
	ptraceotlp.RegisterGRPCServer(ss, h)

	hs.SetServingStatus("jaeger.storage.v2.TraceReader", grpc_health_v1.HealthCheckResponse_SERVING)
	hs.SetServingStatus("jaeger.storage.v2.DependencyReader", grpc_health_v1.HealthCheckResponse_SERVING)
	hs.SetServingStatus("jaeger.storage.v2.TraceWriter", grpc_health_v1.HealthCheckResponse_SERVING)
	hs.SetServingStatus("jaeger.storage.v2.Capabilities", grpc_health_v1.HealthCheckResponse_SERVING)
}

// GetCapabilities answers for the reader this handler fronts, so a client can learn what the
// store behind the remote supports. A reader that cannot determine its own capabilities is
// reported as UNIMPLEMENTED, which clients read as "unknown" rather than as a declaration
// that nothing is supported.
func (h *Handler) GetCapabilities(
	ctx context.Context,
	_ *storage.GetCapabilitiesRequest,
) (*storage.GetCapabilitiesResponse, error) {
	caps, err := h.traceReader.SearchCapabilities(ctx)
	if err != nil {
		if errors.Is(err, errors.ErrUnsupported) {
			return nil, status.Error(codes.Unimplemented, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &storage.GetCapabilitiesResponse{
		Search: &storage.SearchCapabilities{
			WithoutServiceName:  caps.WithoutServiceName,
			SameSpanConjunction: caps.SameSpanConjunction,
			Filter:              toProtoFilterCapabilities(caps.Filter),
		},
	}, nil
}

// toTraceQueryParams prepares a query a third party sent for the reader behind this handler. The
// same three things happen to a query arriving on api_v3, and for the same reasons, so they happen
// through the same checks (RFC 0005 §7):
//
//   - The filter is finalized, because decoding validates nothing: a reader is owed the same tree
//     whether the query came from this process or over the wire.
//   - A query carrying both a filter and the legacy fields it replaces is refused, rather than left
//     for the reader to answer one of them without saying which.
//   - The reader gets whichever filtering model it declared. A reader that evaluates no filter is
//     given the legacy fields instead, which is what keeps a client's filter from reaching one that
//     would ignore the field and answer with every trace in the range.
//
// A refusal is InvalidArgument, because each is something the caller has to change.
func (h *Handler) toTraceQueryParams(
	ctx context.Context,
	t *storage.TraceQueryParameters,
) (tracestore.TraceQueryParams, error) {
	filter, err := expressionproto.FromProto(t.GetFilter())
	if err == nil && filter != nil {
		filter, err = expression.Finalize(filter)
	}
	if err != nil {
		return tracestore.TraceQueryParams{}, status.Error(codes.InvalidArgument, err.Error())
	}
	query := tracestore.TraceQueryParams{
		ServiceName:   t.ServiceName,
		OperationName: t.OperationName,
		Attributes:    convertKeyValueListToMap(t.Attributes),
		StartTimeMin:  t.StartTimeMin,
		StartTimeMax:  t.StartTimeMax,
		DurationMin:   t.DurationMin,
		DurationMax:   t.DurationMax,
		SearchDepth:   int(t.SearchDepth),
		Filter:        filter,
	}
	if err := query.EnsureFilterStandsAlone(); err != nil {
		return tracestore.TraceQueryParams{}, status.Error(codes.InvalidArgument, err.Error())
	}
	if query.Filter == nil {
		return query, nil
	}
	caps, err := h.traceReader.SearchCapabilities(ctx)
	if err != nil {
		// A reader that cannot report its capabilities reads as the least capable one, which serves
		// only the legacy predicate fields.
		caps = tracestore.SearchCapabilities{}
	}
	prepared, err := query.ForCapabilities(caps)
	if err != nil {
		return tracestore.TraceQueryParams{}, status.Error(codes.InvalidArgument, err.Error())
	}
	return prepared, nil
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
	default:
		return // unreachable
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
	default:
		return // unreachable
	}
}
