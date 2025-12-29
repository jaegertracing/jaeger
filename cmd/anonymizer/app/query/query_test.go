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
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	_ "github.com/jaegertracing/jaeger/internal/gogocodec" // force gogo codec registration
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	spanstoremocks "github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/mocks"
)

var (
	matchContext            = mock.AnythingOfType("*context.valueCtx")
	matchGetTraceParameters = mock.AnythingOfType("spanstore.GetTraceParameters")

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
	spanReader spanstore.Reader
}

// GetTrace implements the gRPC GetTrace method by reading from the span reader.
// This is a minimal implementation copied from cmd/query/app/grpc_handler.go
// for the purpose of the anonymizer test.
func (g *testGRPCHandler) GetTrace(r *api_v2.GetTraceRequest, stream api_v2.QueryService_GetTraceServer) error {
	if r.TraceID == (model.TraceID{}) {
		return errUninitializedTraceID
	}
	query := spanstore.GetTraceParameters{
		TraceID:   r.TraceID,
		StartTime: r.StartTime,
		EndTime:   r.EndTime,
	}
	trace, err := g.spanReader.GetTrace(stream.Context(), query)
	if errors.Is(err, spanstore.ErrTraceNotFound) {
		return status.Errorf(codes.NotFound, "%s: %v", msgTraceNotFound, err)
	}
	if err != nil {
		return status.Errorf(codes.Internal, "failed to fetch spans from the backend: %v", err)
	}
	return g.sendSpanChunks(trace.Spans, stream.Send)
}

// sendSpanChunks sends spans in chunks to the client.
// This is copied from cmd/query/app/grpc_handler.go for the purpose of the anonymizer test.
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
	address    net.Addr
	server     *grpc.Server
	spanReader *spanstoremocks.Reader
}

func newTestServer(t *testing.T) *testServer {
	spanReader := &spanstoremocks.Reader{}

	h := &testGRPCHandler{
		spanReader: spanReader,
	}

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
		server:     server,
		address:    lis.Addr(),
		spanReader: spanReader,
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
		expectedGetTraceParameters := spanstore.GetTraceParameters{
			TraceID:   mockTraceID,
			StartTime: startTime,
			EndTime:   endTime,
		}
		s.spanReader.On("GetTrace", matchContext, expectedGetTraceParameters).Return(
			mockTraceGRPC, nil).Once()

		spans, err := q.QueryTrace(mockTraceID.String(), startTime, endTime)
		require.NoError(t, err)
		assert.Len(t, spans, len(mockTraceGRPC.Spans))
	})

	t.Run("Invalid TraceID", func(t *testing.T) {
		_, err := q.QueryTrace(mockInvalidTraceID, time.Time{}, time.Time{})
		assert.ErrorContains(t, err, "failed to convert the provided trace id")
	})

	t.Run("Trace not found", func(t *testing.T) {
		s.spanReader.On("GetTrace", matchContext, matchGetTraceParameters).Return(
			nil, spanstore.ErrTraceNotFound).Once()

		spans, err := q.QueryTrace(mockTraceID.String(), time.Time{}, time.Time{})
		assert.Nil(t, spans)
		assert.ErrorIs(t, err, spanstore.ErrTraceNotFound)
	})
}
