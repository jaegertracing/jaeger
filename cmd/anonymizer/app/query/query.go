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
	"fmt"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// Query represents a jaeger-query's query for trace-id
type Query struct {
	api_v2.QueryServiceClient
	conn   *grpc.ClientConn
	logger *zap.Logger
}

// New creates a Query object
func New(addr string, logger *zap.Logger) (*Query, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr, grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("failed to connect with the jaeger-query service: %w", err)
	}

	return &Query{
		QueryServiceClient: api_v2.NewQueryServiceClient(conn),
		conn:               conn,
		logger:             logger,
	}, nil
}

// QueryTrace queries for a trace and returns all spans inside it
func (q *Query) QueryTrace(traceID string) ([]model.Span, error) {
	mTraceID, err := model.TraceIDFromString(traceID)
	if err != nil {
		return nil, fmt.Errorf("failed to convert the provided trace id: %w", err)
	}

	response, err := q.GetTrace(context.Background(), &api_v2.GetTraceRequest{
		TraceID: mTraceID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch the provided trace id: %w", err)
	}

	spanResponseChunk, err := response.Recv()
	if err == spanstore.ErrTraceNotFound {
		return nil, fmt.Errorf("failed to find the provided trace id: %w", err)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to fetch spans of provided trace id: %w", err)
	}

	spans := spanResponseChunk.GetSpans()
	return spans, nil
}
