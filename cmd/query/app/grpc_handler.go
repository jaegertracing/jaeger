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
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
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

// GetTrace is the GRPC handler to fetch traces.
func (g *GRPCHandler) GetTrace(ctx context.Context, r *api_v2.GetTraceRequest) (*api_v2.GetTraceResponseStream, error) {
	ID := r.GetId()

	trace, err := g.queryService.GetTrace(ctx, ID)
	if err == spanstore.ErrTraceNotFound {
		g.logger.Error("trace not found", zap.Error(err))
		return nil, err
	}
	if err != nil {
		g.logger.Error("Could not fetch spans from backend", zap.Error(err))
		return nil, err
	}

	return &api_v2.GetTraceResponseStream{Spans: trace.Spans}, nil
}

// ArchiveTrace is the GRPC handler to archive traces.
func (g *GRPCHandler) ArchiveTrace(ctx context.Context, r *api_v2.ArchiveTraceRequest) (*api_v2.ArchiveTraceResponse, error) {
	ID := r.GetId()

	err := g.queryService.ArchiveTrace(ctx, ID)
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

// GetServices is the GRPC handler to archive traces.
func (g *GRPCHandler) GetServices(ctx context.Context, r *api_v2.GetServicesRequest) (*api_v2.GetServicesReponse, error) {
	services, err := g.queryService.GetServices(ctx)
	if err != nil {
		g.logger.Error("Error fetching services", zap.Error(err))
		return nil, err
	}

	return &api_v2.GetServicesReponse{services:services}, nil
}

// GetOperations is the GRPC handler to archive traces.
func (g *GRPCHandler) GetOperations(ctx context.Context, r *api_v2.GetOperationsRequest) (*api_v2.GetOperationsReponse, error) {
	service := r.GetService()
	operations, err := g.queryService.GetOperations(ctx, service)
	if err != nil {
		g.logger.Error("Error fetching operations", zap.Error(err))
		return nil, err
	}

	return &api_v2.GetOperationsReponse{operations:operations}, nil
}
