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
