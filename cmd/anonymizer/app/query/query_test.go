// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	_ "github.com/jaegertracing/jaeger/internal/gogocodec" // force gogo codec registration
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
)

var (
	mockInvalidTraceID = "xyz"
	mockTraceID        = model.NewTraceID(0, 123456)

	mockTraceGRPC = &model.Trace{
		Spans: []*model.Span{
			{
				TraceID: mockTraceID,
				SpanID:  model.NewSpanID(1),
				Process: &model.Process{},
			},
			{
				TraceID: mockTraceID,
				SpanID:  model.NewSpanID(2),
				Process: &model.Process{},
			},
		},
		Warnings: []string{},
	}
)

const (
	maxSpanCountInChunk = 10
	msgTraceNotFound    = "trace not found"
)

var errUninitializedTraceID = status.Error(codes.InvalidArgument, "uninitialized TraceID is not allowed")

// testGRPCHandler is a minimal implementation of api_v2.QueryServiceServer
// for testing purposes. It only implements GetTrace, using the embedded
// UnimplementedQueryServiceServer for other methods.
type testGRPCHandler struct {
	api_v2.UnimplementedQueryServiceServer
	returnTrace *model.Trace
	returnError error
}

// GetTrace implements the gRPC GetTrace method by returning test data directly.
func (g *testGRPCHandler) GetTrace(r *api_v2.GetTraceRequest, stream api_v2.QueryService_GetTraceServer) error {
	if r.TraceID == (model.TraceID{}) {
		return errUninitializedTraceID
	}
	if g.returnError != nil {
		if errors.Is(g.returnError, spanstore.ErrTraceNotFound) {
			return status.Errorf(codes.NotFound, "%s: %v", msgTraceNotFound, g.returnError)
		}
		return status.Errorf(codes.Internal, "failed to fetch spans from the backend: %v", g.returnError)
	}
	if g.returnTrace == nil {
		return status.Errorf(codes.NotFound, "%s", msgTraceNotFound)
	}
	return g.sendSpanChunks(g.returnTrace.Spans, stream.Send)
}

// sendSpanChunks sends spans in chunks to the client.
func (g *testGRPCHandler) sendSpanChunks(spans []*model.Span, sendFn func(*api_v2.SpansResponseChunk) error) error {
	chunk := make([]model.Span, 0, len(spans))
	for i := 0; i < len(spans); i += maxSpanCountInChunk {
		chunk = chunk[:0]
		for j := i; j < len(spans) && j < i+maxSpanCountInChunk; j++ {
			chunk = append(chunk, *spans[j])
		}
		if err := sendFn(&api_v2.SpansResponseChunk{Spans: chunk}); err != nil {
			return err
		}
	}
	return nil
}

type testServer struct {
	address net.Addr
	server  *grpc.Server
	handler *testGRPCHandler
}

func newTestServer(t *testing.T) *testServer {
	h := &testGRPCHandler{}

	server := grpc.NewServer()
	api_v2.RegisterQueryServiceServer(server, h)

	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	var started, exited sync.WaitGroup
	started.Add(1)
	exited.Add(1)
	go func() {
		started.Done()
		assert.NoError(t, server.Serve(lis))
		exited.Done()
	}()
	started.Wait()
	t.Cleanup(func() {
		server.Stop()
		exited.Wait() // don't allow test to finish before server exits
	})

	return &testServer{
		server:  server,
		address: lis.Addr(),
		handler: h,
	}
}

func TestNew(t *testing.T) {
	server := newTestServer(t)

	query, err := New(server.address.String())
	require.NoError(t, err)
	defer query.Close()

	assert.NotNil(t, query)
}

func TestQueryTrace(t *testing.T) {
	s := newTestServer(t)
	q, err := New(s.address.String())
	require.NoError(t, err)
	defer q.Close()

	t.Run("No error", func(t *testing.T) {
		startTime := time.Date(1970, time.January, 1, 0, 0, 0, 1000, time.UTC)
		endTime := time.Date(1970, time.January, 1, 0, 0, 0, 2000, time.UTC)
		s.handler.returnTrace = mockTraceGRPC
		s.handler.returnError = nil

		spans, err := q.QueryTrace(mockTraceID.String(), startTime, endTime)
		require.NoError(t, err)
		assert.Len(t, spans, len(mockTraceGRPC.Spans))
	})

	t.Run("Invalid TraceID", func(t *testing.T) {
		_, err := q.QueryTrace(mockInvalidTraceID, time.Time{}, time.Time{})
		assert.ErrorContains(t, err, "failed to convert the provided trace id")
	})

	t.Run("Trace not found", func(t *testing.T) {
		s.handler.returnTrace = nil
		s.handler.returnError = spanstore.ErrTraceNotFound

		spans, err := q.QueryTrace(mockTraceID.String(), time.Time{}, time.Time{})
		assert.Nil(t, spans)
		assert.ErrorIs(t, err, spanstore.ErrTraceNotFound)
	})
}
