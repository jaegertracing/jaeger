// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/cmd/query/app"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/metricstore/disabled"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
	dependencyStoreMocks "github.com/jaegertracing/jaeger/storage_v2/depstore/mocks"
	"github.com/jaegertracing/jaeger/storage_v2/v1adapter"
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

type testServer struct {
	address    net.Addr
	server     *grpc.Server
	spanReader *spanstoremocks.Reader
}

func newTestServer(t *testing.T) *testServer {
	spanReader := &spanstoremocks.Reader{}
	traceReader := v1adapter.NewTraceReader(spanReader)
	metricsReader, err := disabled.NewMetricsReader()
	require.NoError(t, err)

	q := querysvc.NewQueryService(
		traceReader,
		&dependencyStoreMocks.Reader{},
		querysvc.QueryServiceOptions{},
	)
	h := app.NewGRPCHandler(q, metricsReader, app.GRPCHandlerOptions{})

	server := grpc.NewServer()
	api_v2.RegisterQueryServiceServer(server, h)

	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	var exited sync.WaitGroup
	exited.Add(1)
	go func() {
		assert.NoError(t, server.Serve(lis))
		exited.Done()
	}()
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
		assert.Equal(t, len(spans), len(mockTraceGRPC.Spans))
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
