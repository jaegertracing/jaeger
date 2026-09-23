// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

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

// QueryTrace queries for a trace and returns it as one ptrace.Traces. A positive maxSpans stops
// reading the stream once that many spans have arrived and drops the ones beyond it, so a very
// large trace is never held in memory in full.
func (q *Query) QueryTrace(
	traceID pcommon.TraceID,
	startTime time.Time,
	endTime time.Time,
	maxSpans int,
) (ptrace.Traces, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	request := api_v3.GetTraceRequest{
		TraceId:   traceID.String(),
		StartTime: startTime,
		EndTime:   endTime,
	}

	stream, err := q.client.GetTrace(ctx, &request)
	if err != nil {
		return ptrace.Traces{}, unwrapNotFoundErr(err)
	}

	trace := ptrace.NewTraces()
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return trace, nil
		}
		if err != nil {
			return ptrace.Traces{}, unwrapNotFoundErr(err)
		}
		chunk.ToTraces().ResourceSpans().MoveAndAppendTo(trace.ResourceSpans())
		if maxSpans > 0 && trace.SpanCount() >= maxSpans {
			truncate(trace, maxSpans)
			return trace, nil
		}
	}
}

// truncate keeps the first maxSpans spans of the trace in stream order, and drops the scope and
// resource entries left without spans.
func truncate(trace ptrace.Traces, maxSpans int) {
	kept := 0
	trace.ResourceSpans().RemoveIf(func(rs ptrace.ResourceSpans) bool {
		rs.ScopeSpans().RemoveIf(func(ss ptrace.ScopeSpans) bool {
			ss.Spans().RemoveIf(func(ptrace.Span) bool {
				kept++
				return kept > maxSpans
			})
			return ss.Spans().Len() == 0
		})
		return rs.ScopeSpans().Len() == 0
	})
}

// Close closes the grpc client connection
func (q *Query) Close() error {
	return q.conn.Close()
}
