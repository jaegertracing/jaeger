// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/proto-gen/storage/v2"
)

type TraceReader struct {
	client storage.TraceReaderClient
}

func NewTraceReader(tracedConn *grpc.ClientConn) *TraceReader {
	return &TraceReader{
		client: storage.NewTraceReaderClient(tracedConn),
	}
}

func (c *TraceReader) GetTraces(
	ctx context.Context,
	traceIDs ...tracestore.GetTraceParams,
) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		query := []*storage.GetTraceParams{}
		for _, traceID := range traceIDs {
			query = append(query, &storage.GetTraceParams{
				TraceId:   traceID.TraceID[:],
				StartTime: traceID.Start,
				EndTime:   traceID.End,
			})
		}
		stream, err := c.client.GetTraces(ctx, &storage.GetTracesRequest{
			Query: query,
		})
		if err != nil {
			yield(nil, fmt.Errorf("received error from grpc reader client: %w", err))
			return
		}
		for received, err := stream.Recv(); !errors.Is(err, io.EOF); received, err = stream.Recv() {
			if err != nil {
				yield(nil, fmt.Errorf("received error from grpc stream: %w", err))
				return
			}
			traces := []ptrace.Traces{received.ToTraces()}
			yield(traces, nil)
		}
	}
}
