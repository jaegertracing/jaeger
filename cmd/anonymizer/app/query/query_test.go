// Copyright (c) 2020 The Jaeger Authors.
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
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/proto/api_v3"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
)

var mockTraceID = pcommon.TraceID([16]byte{15: 0x40})

const mockTraceIDStr = "40"

// chunk builds one streamed chunk holding a span per name, under a single resource and scope.
func chunk(names ...string) ptrace.Traces {
	traces := ptrace.NewTraces()
	spans := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans()
	for _, name := range names {
		span := spans.AppendEmpty()
		span.SetTraceID(mockTraceID)
		span.SetName(name)
	}
	return traces
}

// testGRPCHandler is a minimal implementation of api_v3.QueryServiceServer
type testGRPCHandler struct {
	api_v3.UnimplementedQueryServiceServer

	chunks  []ptrace.Traces
	err     error
	request *api_v3.GetTraceRequest
}

func (g *testGRPCHandler) GetTrace(r *api_v3.GetTraceRequest, stream api_v3.QueryService_GetTraceServer) error {
	g.request = r
	for _, c := range g.chunks {
		data := jptrace.TracesData(c)
		if err := stream.Send(&data); err != nil {
			return err
		}
	}
	return g.err
}

type testServer struct {
	address net.Addr
	handler *testGRPCHandler
}

func newTestServer(t *testing.T) *testServer {
	h := &testGRPCHandler{}

	server := grpc.NewServer()
	api_v3.RegisterQueryServiceServer(server, h)

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
		address: lis.Addr(),
		handler: h,
	}
}

func newQuery(t *testing.T, s *testServer) *Query {
	q, err := New(s.address.String())
	require.NoError(t, err)
	t.Cleanup(func() { q.Close() })
	return q
}

func spanNames(traces ptrace.Traces) []string {
	var names []string
	for _, rs := range traces.ResourceSpans().All() {
		for _, ss := range rs.ScopeSpans().All() {
			for _, span := range ss.Spans().All() {
				names = append(names, span.Name())
			}
		}
	}
	return names
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
	startTime := time.Date(1970, time.January, 1, 0, 0, 0, 1000, time.UTC)
	endTime := time.Date(1970, time.January, 1, 0, 0, 0, 2000, time.UTC)

	t.Run("chunks are merged into one trace", func(t *testing.T) {
		s := newTestServer(t)
		s.handler.chunks = []ptrace.Traces{chunk("a", "b"), chunk("c")}

		traces, err := newQuery(t, s).QueryTrace(mockTraceIDStr, startTime, endTime)
		require.NoError(t, err)
		assert.Equal(t, []string{"a", "b", "c"}, spanNames(traces))
		assert.Equal(t, 2, traces.ResourceSpans().Len())

		// The ID is sent as given; parsing it is the server's job.
		assert.Equal(t, mockTraceIDStr, s.handler.request.GetTraceId())
		assert.Equal(t, startTime, s.handler.request.GetStartTime())
		assert.Equal(t, endTime, s.handler.request.GetEndTime())
	})

	t.Run("trace not found", func(t *testing.T) {
		s := newTestServer(t)
		s.handler.err = status.Error(codes.NotFound, "trace not found")

		_, err := newQuery(t, s).QueryTrace(mockTraceIDStr, time.Time{}, time.Time{})
		require.ErrorIs(t, err, spanstore.ErrTraceNotFound)
	})

	t.Run("other errors pass through", func(t *testing.T) {
		s := newTestServer(t)
		s.handler.chunks = []ptrace.Traces{chunk("a")}
		s.handler.err = status.Error(codes.Internal, "storage is down")

		_, err := newQuery(t, s).QueryTrace(mockTraceIDStr, time.Time{}, time.Time{})
		require.ErrorContains(t, err, "storage is down")
		require.NotErrorIs(t, err, spanstore.ErrTraceNotFound)
	})
}

type failingQueryClient struct {
	api_v3.QueryServiceClient
}

func (failingQueryClient) GetTrace(context.Context, *api_v3.GetTraceRequest, ...grpc.CallOption) (api_v3.QueryService_GetTraceClient, error) {
	return nil, status.Error(codes.NotFound, "trace not found")
}

func TestQueryTraceStartError(t *testing.T) {
	q := &Query{client: failingQueryClient{}}
	_, err := q.QueryTrace(mockTraceIDStr, time.Time{}, time.Time{})
	require.ErrorIs(t, err, spanstore.ErrTraceNotFound)
}

func TestUnwrapNotFoundErr(t *testing.T) {
	require.ErrorIs(t, unwrapNotFoundErr(status.Error(codes.NotFound, "x")), spanstore.ErrTraceNotFound)
	other := errors.New("other")
	assert.Equal(t, other, unwrapNotFoundErr(other))
}
