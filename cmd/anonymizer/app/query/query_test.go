// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"context"
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

var errUninitializedTraceID = status.Error(codes.InvalidArgument, "uninitialized TraceID is not allowed")

// testGRPCHandler is a minimal implementation of api_v2.QueryServiceServer
// for testing purposes. It only implements GetTrace, using the embedded
// UnimplementedQueryServiceServer for other methods.
type testGRPCHandler struct {
	api_v2.UnimplementedQueryServiceServer
	returnTrace    *model.Trace
	returnError    error
	failDuringRecv bool
}

// GetTrace implements the gRPC GetTrace method by returning test data directly.
func (g *testGRPCHandler) GetTrace(r *api_v2.GetTraceRequest, stream api_v2.QueryService_GetTraceServer) error {
	if r.TraceID == (model.TraceID{}) {
		return errUninitializedTraceID
	}
	if g.returnError != nil {
		if errors.Is(g.returnError, spanstore.ErrTraceNotFound) {
			return status.Errorf(codes.NotFound, "trace not found: %v", g.returnError)
		}
		return status.Errorf(codes.Internal, "failed to fetch spans from the backend: %v", g.returnError)
	}
	if g.returnTrace == nil {
		return status.Errorf(codes.NotFound, "trace not found")
	}
	if g.failDuringRecv {
		// Send first chunk then fail
		chunk := &api_v2.SpansResponseChunk{Spans: []model.Span{*g.returnTrace.Spans[0]}}
		if err := stream.Send(chunk); err != nil {
			return err
		}
		return status.Errorf(codes.Internal, "failed during recv")
	}
	return g.sendSpanChunks(g.returnTrace.Spans, stream.Send)
}

// sendSpanChunks sends spans in chunks to the client.
func (*testGRPCHandler) sendSpanChunks(spans []*model.Span, sendFn func(*api_v2.SpansResponseChunk) error) error {
	chunk := make([]model.Span, 0, len(spans))
	for _, span := range spans {
		chunk = append(chunk, *span)
	}
	return sendFn(&api_v2.SpansResponseChunk{Spans: chunk})
}

type mockQueryClient struct {
	api_v2.QueryServiceClient
	getTraceErr error
}

func (m *mockQueryClient) GetTrace(ctx context.Context, in *api_v2.GetTraceRequest, opts ...grpc.CallOption) (api_v2.QueryService_GetTraceClient, error) {
	if m.getTraceErr != nil {
		return nil, m.getTraceErr
	}
	return m.QueryServiceClient.GetTrace(ctx, in, opts...)
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
	exited.Go(func() {
		started.Done()
		assert.NoError(t, server.Serve(lis))
	})
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

	t.Run("invalid address", func(t *testing.T) {
		// Try a definitively invalid URI to trigger parser error in NewClient.
		q, err := New("invalid-scheme://%%")
		if err != nil {
			assert.Nil(t, q)
		} else if q != nil {
			q.Close()
		}
	})
}

func TestClose(t *testing.T) {
	s := newTestServer(t)
	q, err := New(s.address.String())
	require.NoError(t, err)
	assert.NoError(t, q.Close())
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

	t.Run("General error from GetTrace", func(t *testing.T) {
		s.handler.returnTrace = nil
		s.handler.returnError = errors.New("random error")

		spans, err := q.QueryTrace(mockTraceID.String(), time.Time{}, time.Time{})
		assert.Nil(t, spans)
		assert.ErrorContains(t, err, "random error")
	})

	t.Run("Trace not found", func(t *testing.T) {
		s.handler.returnTrace = nil
		s.handler.returnError = spanstore.ErrTraceNotFound

		spans, err := q.QueryTrace(mockTraceID.String(), time.Time{}, time.Time{})
		assert.Nil(t, spans)
		assert.ErrorIs(t, err, spanstore.ErrTraceNotFound)
	})

	t.Run("Error from GetTrace (immediate)", func(t *testing.T) {
		originalClient := q.client
		mockClient := &mockQueryClient{
			QueryServiceClient: q.client,
			getTraceErr:        errors.New("immediate error"),
		}
		q.client = mockClient
		defer func() { q.client = originalClient }()

		spans, err := q.QueryTrace(mockTraceID.String(), time.Time{}, time.Time{})
		assert.Nil(t, spans)
		assert.ErrorContains(t, err, "immediate error")
	})

	t.Run("Error from stream.Recv", func(t *testing.T) {
		s.handler.returnTrace = mockTraceGRPC
		s.handler.returnError = nil
		s.handler.failDuringRecv = true
		defer func() { s.handler.failDuringRecv = false }()

		spans, err := q.QueryTrace(mockTraceID.String(), time.Time{}, time.Time{})
		assert.Nil(t, spans)
		assert.ErrorContains(t, err, "failed during recv")
	})
}

func TestUnwrapNotFoundErr(t *testing.T) {
	t.Run("non-gRPC error", func(t *testing.T) {
		err := errors.New("standard error")
		assert.Equal(t, err, unwrapNotFoundErr(err))
	})

	t.Run("gRPC error with trace not found", func(t *testing.T) {
		err := status.Error(codes.NotFound, "trace not found")
		assert.Equal(t, spanstore.ErrTraceNotFound, unwrapNotFoundErr(err))
	})

	t.Run("gRPC error without trace not found", func(t *testing.T) {
		err := status.Error(codes.Internal, "internal error")
		assert.Equal(t, err, unwrapNotFoundErr(err))
	})

	t.Run("nil error", func(t *testing.T) {
		assert.NoError(t, unwrapNotFoundErr(nil))
	})
}
