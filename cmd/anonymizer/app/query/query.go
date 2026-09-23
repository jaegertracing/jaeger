// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/proto/api_v3"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
)

// Query represents a jaeger-query's query for trace-id
type Query struct {
	client api_v3.QueryServiceClient
	conn   *grpc.ClientConn
}

// New creates a Query object
func New(addr string) (*Query, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect with the jaeger-query service: %w", err)
	}

	return &Query{
		client: api_v3.NewQueryServiceClient(conn),
		conn:   conn,
	}, nil
}

// unwrapNotFoundErr is a conversion function
func unwrapNotFoundErr(err error) error {
	if status.Code(err) == codes.NotFound {
		return spanstore.ErrTraceNotFound
	}
	return err
}

// QueryTrace queries for a trace and returns it, with the chunks of the stream merged into one
func (q *Query) QueryTrace(traceID string, startTime time.Time, endTime time.Time) (ptrace.Traces, error) {
	pTraceID, err := jptrace.TraceIDFromString(traceID)
	if err != nil {
		return ptrace.Traces{}, fmt.Errorf("failed to convert the provided trace id: %w", err)
	}

	request := api_v3.GetTraceRequest{
		TraceId:   pTraceID.String(),
		StartTime: startTime,
		EndTime:   endTime,
	}

	stream, err := q.client.GetTrace(context.Background(), &request)
	if err != nil {
		return ptrace.Traces{}, unwrapNotFoundErr(err)
	}

	trace := ptrace.NewTraces()
	for received, err := stream.Recv(); !errors.Is(err, io.EOF); received, err = stream.Recv() {
		if err != nil {
			return ptrace.Traces{}, unwrapNotFoundErr(err)
		}
		received.ToTraces().ResourceSpans().MoveAndAppendTo(trace.ResourceSpans())
	}

	return trace, nil
}

// Close closes the grpc client connection
func (q *Query) Close() error {
	return q.conn.Close()
}
