// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"context"
	"iter"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc/v2/querysvc"
	_ "github.com/jaegertracing/jaeger/internal/gogocodec" // force gogo codec registration
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/proto/api_v3"
	dependencyStoreMocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
)

var matchContext = mock.AnythingOfType("*context.valueCtx")

func newGrpcServer(t *testing.T, handler *Handler) (*grpc.Server, net.Addr) {
	server := grpc.NewServer()
	api_v3.RegisterQueryServiceServer(server, handler)

	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	go func() {
		err := server.Serve(lis)
		assert.NoError(t, err)
	}()
	t.Cleanup(func() { server.Stop() })
	return server, lis.Addr()
}

type testServerClient struct {
	server  *grpc.Server
	address net.Addr
	reader  *tracestoremocks.Reader
	client  api_v3.QueryServiceClient
}

func newTestServerClient(t *testing.T) *testServerClient {
	tsc := &testServerClient{
		reader: &tracestoremocks.Reader{},
	}

	q := querysvc.NewQueryService(
		tsc.reader,
		&dependencyStoreMocks.Reader{},
		querysvc.QueryServiceOptions{},
	)
	h := &Handler{
		QueryService: q,
	}
	tsc.server, tsc.address = newGrpcServer(t, h)

	conn, err := grpc.NewClient(
		tsc.address.String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	tsc.client = api_v3.NewQueryServiceClient(conn)

	return tsc
}

func TestGetTrace(t *testing.T) {
	testCases := []struct {
		name          string
		expectedQuery tracestore.GetTraceParams
		request       api_v3.GetTraceRequest
	}{
		{
			"TestGetTrace",
			tracestore.GetTraceParams{
				TraceID: traceID,
				Start:   time.Time{},
				End:     time.Time{},
			},
			api_v3.GetTraceRequest{TraceId: "1"},
		},
		{
			"TestGetTraceWithTimeWindow",
			tracestore.GetTraceParams{
				TraceID: traceID,
				Start:   time.Unix(1, 2).UTC(),
				End:     time.Unix(3, 4).UTC(),
			},
			api_v3.GetTraceRequest{
				TraceId:   "1",
				StartTime: time.Unix(1, 2).UTC(),
				EndTime:   time.Unix(3, 4).UTC(),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tsc := newTestServerClient(t)
			tsc.reader.On("GetTraces", matchContext, tc.expectedQuery).
				Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
					yield([]ptrace.Traces{makeTestTrace()}, nil)
				})).Once()

			getTraceStream, err := tsc.client.GetTrace(context.Background(), &tc.request)
			require.NoError(t, err)
			recv, err := getTraceStream.Recv()
			require.NoError(t, err)
			td := recv.ToTraces()
			require.Equal(t, 1, td.SpanCount())
			assert.Equal(t, "foobar",
				td.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Name())
		})
	}
}

func TestGetTraceStorageError(t *testing.T) {
	tsc := newTestServerClient(t)
	tsc.reader.On("GetTraces", matchContext, tracestore.GetTraceParams{TraceID: traceID}).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield(nil, assert.AnError)
		})).Once()

	getTraceStream, err := tsc.client.GetTrace(context.Background(), &api_v3.GetTraceRequest{
		TraceId: "1",
	})
	require.NoError(t, err)
	recv, err := getTraceStream.Recv()
	require.ErrorContains(t, err, assert.AnError.Error())
	assert.Nil(t, recv)
}

func TestGetTraceTraceIDError(t *testing.T) {
	tsc := newTestServerClient(t)
	tsc.reader.On("GetTraces", matchContext, mock.AnythingOfType("tracestore.GetTraceParams")).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield([]ptrace.Traces{}, nil)
		})).Once()

	getTraceStream, err := tsc.client.GetTrace(context.Background(), &api_v3.GetTraceRequest{
		TraceId:   "Z",
		StartTime: time.Now().Add(-2 * time.Hour),
		EndTime:   time.Now(),
	})
	require.NoError(t, err)
	recv, err := getTraceStream.Recv()
	require.ErrorContains(t, err, "strconv.ParseUint:")
	assert.Nil(t, recv)
}

func TestFindTraces(t *testing.T) {
	tsc := newTestServerClient(t)
	tsc.reader.On("FindTraces", matchContext, mock.AnythingOfType("tracestore.TraceQueryParams")).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield([]ptrace.Traces{makeTestTrace()}, nil)
		})).Once()

	responseStream, err := tsc.client.FindTraces(context.Background(), &api_v3.FindTracesRequest{
		Query: &api_v3.TraceQueryParameters{
			ServiceName:   "myservice",
			OperationName: "foobar",
			Attributes:    map[string]string{"foo": "bar"},
			StartTimeMin:  time.Now().Add(-2 * time.Hour),
			StartTimeMax:  time.Now(),
			DurationMin:   1 * time.Second,
			DurationMax:   2 * time.Second,
			SearchDepth:   10,
		},
	})
	require.NoError(t, err)
	recv, err := responseStream.Recv()
	require.NoError(t, err)
	td := recv.ToTraces()
	require.Equal(t, 1, td.SpanCount())
}

func TestFindTracesSendError(t *testing.T) {
	reader := new(tracestoremocks.Reader)
	reader.On("FindTraces", mock.Anything, mock.AnythingOfType("tracestore.TraceQueryParams")).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield([]ptrace.Traces{makeTestTrace()}, nil)
		})).Once()
	h := &Handler{
		QueryService: querysvc.NewQueryService(
			reader,
			new(dependencyStoreMocks.Reader),
			querysvc.QueryServiceOptions{},
		),
	}
	err := h.internalFindTraces(context.Background(),
		&api_v3.FindTracesRequest{
			Query: &api_v3.TraceQueryParameters{
				StartTimeMin: time.Now().Add(-2 * time.Hour),
				StartTimeMax: time.Now(),
			},
		},
		/* streamSend= */ func(*jptrace.TracesData) error {
			return assert.AnError
		},
	)
	require.ErrorContains(t, err, assert.AnError.Error())
	require.ErrorContains(t, err, "failed to send response")
}

func TestFindTracesQueryNil(t *testing.T) {
	tsc := newTestServerClient(t)
	responseStream, err := tsc.client.FindTraces(context.Background(), &api_v3.FindTracesRequest{})
	require.NoError(t, err)
	recv, err := responseStream.Recv()
	require.ErrorContains(t, err, "missing query")
	assert.Nil(t, recv)

	responseStream, err = tsc.client.FindTraces(context.Background(), &api_v3.FindTracesRequest{
		Query: &api_v3.TraceQueryParameters{},
	})
	require.NoError(t, err)
	recv, err = responseStream.Recv()
	require.ErrorContains(t, err, "start time min and max are required parameters")
	assert.Nil(t, recv)
}

func TestFindTracesStorageError(t *testing.T) {
	tsc := newTestServerClient(t)
	tsc.reader.On("FindTraces", matchContext, mock.AnythingOfType("tracestore.TraceQueryParams")).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield(nil, assert.AnError)
		})).Once()

	responseStream, err := tsc.client.FindTraces(context.Background(), &api_v3.FindTracesRequest{
		Query: &api_v3.TraceQueryParameters{
			StartTimeMin: time.Now().Add(-2 * time.Hour),
			StartTimeMax: time.Now(),
		},
	})
	require.NoError(t, err)
	recv, err := responseStream.Recv()
	require.ErrorContains(t, err, assert.AnError.Error())
	assert.Nil(t, recv)
}

func TestGetServices(t *testing.T) {
	tsc := newTestServerClient(t)
	tsc.reader.On("GetServices", matchContext).Return(
		[]string{"foo"}, nil).Once()

	response, err := tsc.client.GetServices(context.Background(), &api_v3.GetServicesRequest{})
	require.NoError(t, err)
	assert.Equal(t, []string{"foo"}, response.GetServices())
}

func TestGetServicesStorageError(t *testing.T) {
	tsc := newTestServerClient(t)
	tsc.reader.On("GetServices", matchContext).Return(
		nil, assert.AnError).Once()

	response, err := tsc.client.GetServices(context.Background(), &api_v3.GetServicesRequest{})
	require.ErrorContains(t, err, assert.AnError.Error())
	assert.Nil(t, response)
}

func TestGetOperations(t *testing.T) {
	tsc := newTestServerClient(t)
	tsc.reader.On("GetOperations", matchContext, mock.AnythingOfType("tracestore.OperationQueryParams")).Return(
		[]tracestore.Operation{
			{
				Name: "get_users",
			},
		}, nil).Once()

	response, err := tsc.client.GetOperations(context.Background(), &api_v3.GetOperationsRequest{})
	require.NoError(t, err)
	assert.Equal(t, []*api_v3.Operation{
		{
			Name: "get_users",
		},
	}, response.GetOperations())
}

func TestGetOperationsStorageError(t *testing.T) {
	tsc := newTestServerClient(t)
	tsc.reader.On("GetOperations", matchContext, mock.AnythingOfType("tracestore.OperationQueryParams")).Return(
		nil, assert.AnError).Once()

	response, err := tsc.client.GetOperations(context.Background(), &api_v3.GetOperationsRequest{})
	require.ErrorContains(t, err, assert.AnError.Error())
	assert.Nil(t, response)
}
