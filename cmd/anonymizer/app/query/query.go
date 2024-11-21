// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
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
		if strings.Contains(s.Message(), spanstore.ErrTraceNotFound.Error()) {
			return spanstore.ErrTraceNotFound
		}
	}
	return err
}

// QueryTrace queries for a trace and returns all spans inside it
func (q *Query) QueryTrace(traceID string, startTime int64, endTime int64) ([]model.Span, error) {
	mTraceID, err := model.TraceIDFromString(traceID)
	if err != nil {
		return nil, fmt.Errorf("failed to convert the provided trace id: %w", err)
	}

	request := api_v2.GetTraceRequest{
		TraceID: mTraceID,
	}

	if startTime != -1 {
		request.StartTime = time.UnixMicro(startTime)
	}

	if endTime != -1 {
		request.EndTime = time.UnixMicro(endTime)
	}

	stream, err := q.client.GetTrace(context.Background(), &request)
	if err != nil {
		return nil, unwrapNotFoundErr(err)
	}

	var spans []model.Span
	for received, err := stream.Recv(); !errors.Is(err, io.EOF); received, err = stream.Recv() {
		if err != nil {
			return nil, unwrapNotFoundErr(err)
		}
		spans = append(spans, received.Spans...)
	}

	return spans, nil
}

// Close closes the grpc client connection
func (q *Query) Close() error {
	return q.conn.Close()
}
