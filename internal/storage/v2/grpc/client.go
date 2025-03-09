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

	"github.com/jaegertracing/jaeger/proto-gen/storage/v2"
)

type Client struct {
	readerClient storage.TraceReaderClient
}

func NewClient(tracedConn *grpc.ClientConn) *Client {
	return &Client{
		readerClient: storage.NewTraceReaderClient(tracedConn),
	}
}

func (c *Client) GetTraces(ctx context.Context,
	traceIDs ...storage.GetTraceParams,
) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		query := []*storage.GetTraceParams{}
		for _, traceID := range traceIDs {
			query = append(query, &traceID)
		}
		stream, err := c.readerClient.GetTraces(ctx, &storage.GetTracesRequest{
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
