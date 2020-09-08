// Copyright (c) 2019 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"context"

	"github.com/opentracing/opentracing-go"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	maxSpanCountInChunk = 10

	msgTraceNotFound = "trace not found"
)

// GRPCHandler implements the gRPC endpoint of the query service.
type GRPCHandler struct {
	queryService *querysvc.QueryService
	logger       *zap.Logger
	tracer       opentracing.Tracer
}

// NewGRPCHandler returns a GRPCHandler
func NewGRPCHandler(queryService *querysvc.QueryService, logger *zap.Logger, tracer opentracing.Tracer) *GRPCHandler {
	gH := &GRPCHandler{
		queryService: queryService,
		logger:       logger,
		tracer:       tracer,
	}

	return gH
}

// GetTrace is the gRPC handler to fetch traces based on trace-id.
func (g *GRPCHandler) GetTrace(r *api_v2.GetTraceRequest, stream api_v2.QueryService_GetTraceServer) error {
	trace, err := g.queryService.GetTrace(stream.Context(), r.TraceID)
	if err == spanstore.ErrTraceNotFound {
		g.logger.Error(msgTraceNotFound, zap.Error(err))
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
	err := g.queryService.ArchiveTrace(ctx, r.TraceID)
	if err == spanstore.ErrTraceNotFound {
		g.logger.Error("trace not found", zap.Error(err))
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
	query := r.GetQuery()
	queryParams := spanstore.TraceQueryParameters{
		ServiceName:   query.ServiceName,
		OperationName: query.OperationName,
		Tags:          query.Tags,
		StartTimeMin:  query.StartTimeMin,
		StartTimeMax:  query.StartTimeMax,
		DurationMin:   query.DurationMin,
		DurationMax:   query.DurationMax,
		NumTraces:     int(query.SearchDepth),
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
func (g *GRPCHandler) GetServices(ctx context.Context, r *api_v2.GetServicesRequest) (*api_v2.GetServicesResponse, error) {
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
	startTime := r.StartTime
	endTime := r.EndTime
	dependencies, err := g.queryService.GetDependencies(ctx, startTime, endTime.Sub(startTime))
	if err != nil {
		g.logger.Error("failed to fetch dependencies", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to fetch dependencies: %v", err)
	}

	return &api_v2.GetDependenciesResponse{Dependencies: dependencies}, nil
}
