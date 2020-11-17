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
	"io"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// Query represents a jaeger-query's query for trace-id
type Query struct {
	client api_v2.QueryServiceClient
	conn   *grpc.ClientConn
}

// New creates a Query object
func New(addr string) (*Query, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr, grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("failed to connect with the jaeger-query service: %w", err)
	}

	return &Query{
		client: api_v2.NewQueryServiceClient(conn),
		conn:   conn,
	}, nil
}

// unwrapNotFoundErr is a conversion function
func unwrapNotFoundErr(err error) error {
	if s, _ := status.FromError(err); s != nil {
		if s.Message() == spanstore.ErrTraceNotFound.Error() {
			return spanstore.ErrTraceNotFound
		}
	}
	return err
}

// QueryTrace queries for a trace and returns all spans inside it
func (q *Query) QueryTrace(traceID string) ([]model.Span, error) {
	mTraceID, err := model.TraceIDFromString(traceID)
	if err != nil {
		return nil, fmt.Errorf("failed to convert the provided trace id: %w", err)
	}

	stream, err := q.client.GetTrace(context.Background(), &api_v2.GetTraceRequest{
		TraceID: mTraceID,
	})
	if err != nil {
		return nil, unwrapNotFoundErr(err)
	}

	var spans []model.Span
	for received, err := stream.Recv(); err != io.EOF; received, err = stream.Recv() {
		if err != nil {
			return nil, unwrapNotFoundErr(err)
		}
		for i := range received.Spans {
			spans = append(spans, received.Spans[i])
		}
	}

	return spans, nil
}
