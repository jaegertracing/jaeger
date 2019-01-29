// Copyright (c) 2019 Jaegertracing Authors.
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
	"net/http"
	"strings"
	"time"

	"github.com/opentracing/opentracing-go"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// GRPCHandler implements the GRPC endpoint of the query service.
type GRPCHandler struct {
	spanReader        spanstore.Reader
	archiveSpanReader spanstore.Reader
	dependencyReader  dependencystore.Reader
	logger            *zap.Logger
}

// NewGRPCHandler returns a GRPCHandler
func NewGRPCHandler(spanReader spanstore.Reader, dependencyReader dependencystore.Reader, options ...HandlerOption) *GRPCHandler {
	gH := &GRPCHandler{
		spanReader:       spanReader,
		dependencyReader: dependencyReader,
	}

	if gH.logger == nil {
		gH.logger = zap.NewNop()
	}

	return gH
}

// NewCombinedHandler returns a handler where GRPC and HTTP are multiplexed.
func NewCombinedHandler(grpcServer *grpc.Server, recoveryHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.Contains(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
		} else {
			recoveryHandler.ServeHTTP(w, r)
		}
	})
}

// GetTrace is the GRPC handler to fetch traces.
func (g *GRPCHandler) GetTrace(ctx context.Context, r *api_v2.GetTraceRequest) (*api_v2.GetTraceResponse, error) {
	ID := r.GetId()

	trace, err := g.spanReader.GetTrace(ctx, ID)
	if err == spanstore.ErrTraceNotFound {
		if g.archiveSpanReader == nil {
			return &api_v2.GetTraceResponse{}, err
		}

		trace, err = g.archiveSpanReader.GetTrace(ctx, traceID)
		if err != nil {
			g.logger.Error("Could not fetch spans from backend", zap.Error(err))
		}
	}

	return &api_v2.GetTraceResponse{Trace: trace}, nil
}
