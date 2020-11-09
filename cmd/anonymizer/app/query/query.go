// Copyright (c) 2020 The Jaeger Authors.
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

package query

import (
	"context"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// GrpcClient represents a grpc-client to jaeger-query
type GrpcClient struct {
	api_v2.QueryServiceClient
	conn *grpc.ClientConn
}

// Query represents a jaeger-query's query for trace-id
type Query struct {
	grpcClient *GrpcClient
	logger     *zap.Logger
}

// newGRPCClient returns a new GrpcClient
func newGRPCClient(addr string, logger *zap.Logger) *GrpcClient {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr, grpc.WithInsecure())
	if err != nil {
		logger.Fatal("failed to connect with the jaeger-query service", zap.Error(err))
	}

	return &GrpcClient{
		QueryServiceClient: api_v2.NewQueryServiceClient(conn),
		conn:               conn,
	}
}

// New creates a Query object
func New(addr string, logger *zap.Logger) (*Query, error) {
	client := newGRPCClient(addr, logger)

	return &Query{
		grpcClient: client,
		logger:     logger,
	}, nil
}

// QueryTrace queries for a trace and returns all spans inside it
func (q *Query) QueryTrace(traceID string) []model.Span {
	mTraceID, err := model.TraceIDFromString(traceID)
	if err != nil {
		q.logger.Fatal("failed to convert the provided trace id", zap.Error(err))
	}

	response, err := q.grpcClient.GetTrace(context.Background(), &api_v2.GetTraceRequest{
		TraceID: mTraceID,
	})
	if err != nil {
		q.logger.Fatal("failed to fetch the provided trace id", zap.Error(err))
	}

	spanResponseChunk, err := response.Recv()
	if err == spanstore.ErrTraceNotFound {
		q.logger.Fatal("failed to find the provided trace id", zap.Error(err))
	}
	if err != nil {
		q.logger.Fatal("failed to fetch spans of provided trace id", zap.Error(err))
	}

	spans := spanResponseChunk.GetSpans()
	return spans
}
