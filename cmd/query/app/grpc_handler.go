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

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// GRPCHandler implements the GRPC endpoint of the query service.
type GRPCHandler struct {
	queryService querysvc.QueryService
	logger       *zap.Logger
	tracer       opentracing.Tracer
}

// NewGRPCHandler returns a GRPCHandler
func NewGRPCHandler(queryService querysvc.QueryService, logger *zap.Logger, tracer opentracing.Tracer) *GRPCHandler {
	gH := &GRPCHandler{
		queryService: queryService,
		logger:       logger,
		tracer:       tracer,
	}

	return gH
}

// GetTrace is the GRPC handler to fetch traces based on trace-id.
func (g *GRPCHandler) GetTrace(r *api_v2.GetTraceRequest, w api_v2.QueryService_GetTraceServer) error {
	trace, err := g.queryService.GetTrace(ctx, r.TraceID)
	if err == spanstore.ErrTraceNotFound {
		g.logger.Error("trace not found", zap.Error(err))
		return nil, err
	}
	if err != nil {
		g.logger.Error("Could not fetch spans from backend", zap.Error(err))
		return nil, err
	}

	spans := make([]model.Span, 0, len(trace.Spans))
	for _, span := range trace.Spans {
		spans = append(spans, *span)
	}

	return &api_v2.SpansResponseChunk{Spans: spans}, nil
}

// ArchiveTrace is the GRPC handler to archive traces.
func (g *GRPCHandler) ArchiveTrace(ctx context.Context, r *api_v2.ArchiveTraceRequest) (*api_v2.ArchiveTraceResponse, error) {
	err := g.queryService.ArchiveTrace(ctx, r.TraceID)
	if err == spanstore.ErrTraceNotFound {
		g.logger.Error("trace not found", zap.Error(err))
		return nil, err
	}
	if err != nil {
		g.logger.Error("Could not fetch spans from backend", zap.Error(err))
		return nil, err
	}

	return &api_v2.ArchiveTraceResponse{}, nil
}

// FindTraces is the GRPC handler to fetch traces based on TraceQueryParameters.
func (g *GRPCHandler) FindTraces(ctx context.Context, r *api_v2.FindTracesRequest) (*api_v2.SpansResponseChunk, error) {
	query := r.GetQuery()
	queryParams := spanstore.TraceQueryParameters{
		ServiceName:   query.ServiceName,
		OperationName: query.OperationName,
		Tags:          query.Tags,
		StartTimeMin:  query.StartTimeMin,
		StartTimeMax:  query.StartTimeMax,
		DurationMin:   query.DurationMin,
		DurationMax:   query.DurationMax,
		NumTraces:     int(query.NumTraces),
	}
	traces, err := g.queryService.FindTraces(ctx, &queryParams)
	if err != nil {
		g.logger.Error("Error fetching traces", zap.Error(err))
		return nil, err
	}

	spans := []model.Span{}
	for _, trace := range traces {
		for _, span := range trace.Spans {
			spans = append(spans, *span)
		}
	}
	return &api_v2.SpansResponseChunk{Spans: spans}, nil
}

// GetServices is the GRPC handler to fetch services.
func (g *GRPCHandler) GetServices(ctx context.Context, r *api_v2.GetServicesRequest) (*api_v2.GetServicesResponse, error) {
	services, err := g.queryService.GetServices(ctx)
	if err != nil {
		g.logger.Error("Error fetching services", zap.Error(err))
		return nil, err
	}

	return &api_v2.GetServicesResponse{Services: services}, nil
}

// GetOperations is the GRPC handler to fetch operations.
func (g *GRPCHandler) GetOperations(ctx context.Context, r *api_v2.GetOperationsRequest) (*api_v2.GetOperationsResponse, error) {
	service := r.Service
	operations, err := g.queryService.GetOperations(ctx, service)
	if err != nil {
		g.logger.Error("Error fetching operations", zap.Error(err))
		return nil, err
	}

	return &api_v2.GetOperationsResponse{Operations: operations}, nil
}

// GetDependencies is the GRPC handler to fetch dependencies.
func (g *GRPCHandler) GetDependencies(ctx context.Context, r *api_v2.GetDependenciesRequest) (*api_v2.GetDependenciesResponse, error) {
	startTime := r.StartTime
	endTime := r.EndTime
	dependencies, err := g.queryService.GetDependencies(startTime, endTime.Sub(startTime))
	if err != nil {
		g.logger.Error("Error fetching dependencies", zap.Error(err))
		return nil, err
	}

	return &api_v2.GetDependenciesResponse{Dependencies: dependencies}, nil
}
