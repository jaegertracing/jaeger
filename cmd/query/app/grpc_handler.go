// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	_ "github.com/jaegertracing/jaeger/pkg/gogocodec" // force gogo codec registration
	"github.com/jaegertracing/jaeger/pkg/jtracer"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	maxSpanCountInChunk = 10

	msgTraceNotFound = "trace not found"
)

var (
	errNilRequest           = status.Error(codes.InvalidArgument, "a nil argument is not allowed")
	errUninitializedTraceID = status.Error(codes.InvalidArgument, "uninitialized TraceID is not allowed")
	errMissingServiceNames  = status.Error(codes.InvalidArgument, "please provide at least one service name")
	errMissingQuantile      = status.Error(codes.InvalidArgument, "please provide a quantile between (0, 1]")
)

// GRPCHandler implements the gRPC endpoint of the query service.
type GRPCHandler struct {
	queryService *querysvc.QueryService
	logger       *zap.Logger
	tracer       *jtracer.JTracer
	nowFn        func() time.Time
}

// GRPCHandlerOptions contains optional members of GRPCHandler.
type GRPCHandlerOptions struct {
	Logger *zap.Logger
	Tracer *jtracer.JTracer
	NowFn  func() time.Time
}

// NewGRPCHandler returns a GRPCHandler.
func NewGRPCHandler(queryService *querysvc.QueryService,
	options GRPCHandlerOptions,
) *GRPCHandler {
	if options.Logger == nil {
		options.Logger = zap.NewNop()
	}

	if options.Tracer == nil {
		options.Tracer = jtracer.NoOp()
	}

	if options.NowFn == nil {
		options.NowFn = time.Now
	}

	return &GRPCHandler{
		queryService: queryService,
		logger:       options.Logger,
		tracer:       options.Tracer,
		nowFn:        options.NowFn,
	}
}

var _ api_v2.QueryServiceServer = (*GRPCHandler)(nil)

// GetTrace is the gRPC handler to fetch traces based on trace-id.
func (g *GRPCHandler) GetTrace(r *api_v2.GetTraceRequest, stream api_v2.QueryService_GetTraceServer) error {
	if r == nil {
		return errNilRequest
	}
	if r.TraceID == (model.TraceID{}) {
		return errUninitializedTraceID
	}
	query := querysvc.GetTraceParameters{
		GetTraceParameters: spanstore.GetTraceParameters{
			TraceID:   r.TraceID,
			StartTime: r.StartTime,
			EndTime:   r.EndTime,
		},
		RawTraces: r.RawTraces,
	}
	trace, err := g.queryService.GetTrace(stream.Context(), query)
	if errors.Is(err, spanstore.ErrTraceNotFound) {
		g.logger.Warn(msgTraceNotFound, zap.Stringer("id", r.TraceID), zap.Error(err))
		return status.Errorf(codes.NotFound, "%s: %v", msgTraceNotFound, err)
	}
	if err != nil {
		g.logger.Error("failed to fetch spans from the backend", zap.Error(err))
		return status.Errorf(codes.Internal, "failed to fetch spans from the backend: %v", err)
	}
	return g.sendSpanChunks(trace.Spans, stream.Send)
}

// ArchiveTrace is the gRPC handler to archive traces.
func (g *GRPCHandler) ArchiveTrace(ctx context.Context, r *api_v2.ArchiveTraceRequest) (*api_v2.ArchiveTraceResponse, error) {
	if r == nil {
		return nil, errNilRequest
	}
	if r.TraceID == (model.TraceID{}) {
		return nil, errUninitializedTraceID
	}
	query := spanstore.GetTraceParameters{
		TraceID:   r.TraceID,
		StartTime: r.StartTime,
		EndTime:   r.EndTime,
	}

	err := g.queryService.ArchiveTrace(ctx, query)
	if errors.Is(err, spanstore.ErrTraceNotFound) {
		g.logger.Warn(msgTraceNotFound, zap.Stringer("id", r.TraceID), zap.Error(err))
		return nil, status.Errorf(codes.NotFound, "%s: %v", msgTraceNotFound, err)
	}
	if err != nil {
		g.logger.Error("failed to archive trace", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to archive trace: %v", err)
	}

	return &api_v2.ArchiveTraceResponse{}, nil
}

// FindTraces is the gRPC handler to fetch traces based on TraceQueryParameters.
func (g *GRPCHandler) FindTraces(r *api_v2.FindTracesRequest, stream api_v2.QueryService_FindTracesServer) error {
	if r == nil {
		return errNilRequest
	}
	query := r.GetQuery()
	if query == nil {
		return status.Errorf(codes.InvalidArgument, "missing query")
	}
	queryParams := querysvc.TraceQueryParameters{
		TraceQueryParameters: spanstore.TraceQueryParameters{
			ServiceName:   query.ServiceName,
			OperationName: query.OperationName,
			Tags:          query.Tags,
			StartTimeMin:  query.StartTimeMin,
			StartTimeMax:  query.StartTimeMax,
			DurationMin:   query.DurationMin,
			DurationMax:   query.DurationMax,
			NumTraces:     int(query.SearchDepth),
		},
		RawTraces: query.RawTraces,
	}
	traces, err := g.queryService.FindTraces(stream.Context(), &queryParams)
	if err != nil {
		g.logger.Error("failed when searching for traces", zap.Error(err))
		return status.Errorf(codes.Internal, "failed when searching for traces: %v", err)
	}
	for _, trace := range traces {
		if err := g.sendSpanChunks(trace.Spans, stream.Send); err != nil {
			return err
		}
	}
	return nil
}

func (g *GRPCHandler) sendSpanChunks(spans []*model.Span, sendFn func(*api_v2.SpansResponseChunk) error) error {
	chunk := make([]model.Span, 0, len(spans))
	for i := 0; i < len(spans); i += maxSpanCountInChunk {
		chunk = chunk[:0]
		for j := i; j < len(spans) && j < i+maxSpanCountInChunk; j++ {
			chunk = append(chunk, *spans[j])
		}
		if err := sendFn(&api_v2.SpansResponseChunk{Spans: chunk}); err != nil {
			g.logger.Error("failed to send response to client", zap.Error(err))
			return err
		}
	}
	return nil
}

// GetServices is the gRPC handler to fetch services.
func (g *GRPCHandler) GetServices(ctx context.Context, _ *api_v2.GetServicesRequest) (*api_v2.GetServicesResponse, error) {
	services, err := g.queryService.GetServices(ctx)
	if err != nil {
		g.logger.Error("failed to fetch services", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to fetch services: %v", err)
	}

	return &api_v2.GetServicesResponse{Services: services}, nil
}

// GetOperations is the gRPC handler to fetch operations.
func (g *GRPCHandler) GetOperations(
	ctx context.Context,
	r *api_v2.GetOperationsRequest,
) (*api_v2.GetOperationsResponse, error) {
	if r == nil {
		return nil, errNilRequest
	}
	operations, err := g.queryService.GetOperations(ctx, spanstore.OperationQueryParameters{
		ServiceName: r.Service,
		SpanKind:    r.SpanKind,
	})
	if err != nil {
		g.logger.Error("failed to fetch operations", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to fetch operations: %v", err)
	}

	result := make([]*api_v2.Operation, len(operations))
	for i, operation := range operations {
		result[i] = &api_v2.Operation{
			Name:     operation.Name,
			SpanKind: operation.SpanKind,
		}
	}
	return &api_v2.GetOperationsResponse{
		Operations: result,
		// TODO: remove OperationNames after all clients are updated
		OperationNames: getUniqueOperationNames(operations),
	}, nil
}

// GetDependencies is the gRPC handler to fetch dependencies.
func (g *GRPCHandler) GetDependencies(ctx context.Context, r *api_v2.GetDependenciesRequest) (*api_v2.GetDependenciesResponse, error) {
	if r == nil {
		return nil, errNilRequest
	}

	startTime := r.StartTime
	endTime := r.EndTime
	if startTime == (time.Time{}) || endTime == (time.Time{}) {
		return nil, status.Errorf(codes.InvalidArgument, "StartTime and EndTime must be initialized.")
	}

	dependencies, err := g.queryService.GetDependencies(ctx, endTime, endTime.Sub(startTime))
	if err != nil {
		g.logger.Error("failed to fetch dependencies", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to fetch dependencies: %v", err)
	}

	return &api_v2.GetDependenciesResponse{Dependencies: dependencies}, nil
}

func (g *GRPCHandler) handleErr(msg string, err error) error {
	if err == nil {
		return nil
	}
	g.logger.Error(msg, zap.Error(err))

	if _, ok := status.FromError(err); ok {
		return err
	}

	// Received an "unexpected" error.
	return status.Errorf(codes.Internal, "%s: %v", msg, err)
}
