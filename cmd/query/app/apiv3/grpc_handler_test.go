// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/jaegertracing/jaeger/cmd/query/app/internal/api_v3"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/model"
	_ "github.com/jaegertracing/jaeger/pkg/gogocodec" // force gogo codec registration
	dependencyStoreMocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

var (
	matchContext            = mock.AnythingOfType("*context.valueCtx")
	matchTraceGetParameters = mock.AnythingOfType("spanstore.TraceGetParameters")
)

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
	reader  *spanstoremocks.Reader
	client  api_v3.QueryServiceClient
}

func newTestServerClient(t *testing.T) *testServerClient {
	tsc := &testServerClient{
		reader: &spanstoremocks.Reader{},
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
	tsc := newTestServerClient(t)
	traceIdLiteral := "156"
	traceId, _ := model.TraceIDFromString(traceIdLiteral)
	startTime := types.Timestamp{Seconds: 1, Nanos: 2}
	endTime := types.Timestamp{Seconds: 3, Nanos: 4}

	traceGetStartTime := time.Unix(startTime.GetSeconds(), int64(startTime.GetNanos()))
	traceGetEndTime := time.Unix(endTime.GetSeconds(), int64(endTime.GetNanos()))

	expectedTraceGetParameters := spanstore.TraceGetParameters{
		TraceID:   traceId,
		StartTime: &traceGetStartTime,
		EndTime:   &traceGetEndTime,
	}
	tsc.reader.On("GetTrace", matchContext, expectedTraceGetParameters).Return(
		&model.Trace{
			Spans: []*model.Span{
				{
					OperationName: "foobar",
				},
			},
		}, nil).Once()

	getTraceStream, err := tsc.client.GetTrace(context.Background(),
		&api_v3.GetTraceRequest{
			TraceId:   traceIdLiteral,
			StartTime: &startTime,
			EndTime:   &endTime,
		},
	)
	require.NoError(t, err)
	recv, err := getTraceStream.Recv()
	require.NoError(t, err)
	td := recv.ToTraces()
	require.EqualValues(t, 1, td.SpanCount())
	assert.Equal(t, "foobar",
		td.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Name())
}

func TestGetTraceStorageError(t *testing.T) {
	tsc := newTestServerClient(t)
	tsc.reader.On("GetTrace", matchContext, matchTraceGetParameters).Return(
		nil, fmt.Errorf("storage_error")).Once()

	getTraceStream, err := tsc.client.GetTrace(context.Background(), &api_v3.GetTraceRequest{
		TraceId: "156",
	})
	require.NoError(t, err)
	recv, err := getTraceStream.Recv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage_error")
	assert.Nil(t, recv)
}

func TestGetTraceTraceIDError(t *testing.T) {
	tsc := newTestServerClient(t)
	tsc.reader.On("GetTrace", matchContext, matchTraceGetParameters).Return(
		&model.Trace{
			Spans: []*model.Span{},
		}, nil).Once()

	getTraceStream, err := tsc.client.GetTrace(context.Background(), &api_v3.GetTraceRequest{
		TraceId: "Z",
	})
	require.NoError(t, err)
	recv, err := getTraceStream.Recv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "strconv.ParseUint:")
	assert.Nil(t, recv)
}

func TestFindTraces(t *testing.T) {
	tsc := newTestServerClient(t)
	tsc.reader.On("FindTraces", matchContext, mock.AnythingOfType("*spanstore.TraceQueryParameters")).Return(
		[]*model.Trace{
			{
				Spans: []*model.Span{
					{
						OperationName: "name",
					},
				},
			},
		}, nil).Once()

	responseStream, err := tsc.client.FindTraces(context.Background(), &api_v3.FindTracesRequest{
		Query: &api_v3.TraceQueryParameters{
			ServiceName:   "myservice",
			OperationName: "opname",
			Attributes:    map[string]string{"foo": "bar"},
			StartTimeMin:  &types.Timestamp{},
			StartTimeMax:  &types.Timestamp{},
			DurationMin:   &types.Duration{},
			DurationMax:   &types.Duration{},
		},
	})
	require.NoError(t, err)
	recv, err := responseStream.Recv()
	require.NoError(t, err)
	td := recv.ToTraces()
	require.EqualValues(t, 1, td.SpanCount())
}

func TestFindTracesQueryNil(t *testing.T) {
	tsc := newTestServerClient(t)
	responseStream, err := tsc.client.FindTraces(context.Background(), &api_v3.FindTracesRequest{})
	require.NoError(t, err)
	recv, err := responseStream.Recv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing query")
	assert.Nil(t, recv)

	responseStream, err = tsc.client.FindTraces(context.Background(), &api_v3.FindTracesRequest{
		Query: &api_v3.TraceQueryParameters{
			StartTimeMin: nil,
			StartTimeMax: nil,
		},
	})
	require.NoError(t, err)
	recv, err = responseStream.Recv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start time min and max are required parameters")
	assert.Nil(t, recv)
}

func TestFindTracesStorageError(t *testing.T) {
	tsc := newTestServerClient(t)
	tsc.reader.On("FindTraces", matchContext, mock.AnythingOfType("*spanstore.TraceQueryParameters")).Return(
		nil, fmt.Errorf("storage_error"), nil).Once()

	responseStream, err := tsc.client.FindTraces(context.Background(), &api_v3.FindTracesRequest{
		Query: &api_v3.TraceQueryParameters{
			StartTimeMin: &types.Timestamp{},
			StartTimeMax: &types.Timestamp{},
			DurationMin:  &types.Duration{},
			DurationMax:  &types.Duration{},
		},
	})
	require.NoError(t, err)
	recv, err := responseStream.Recv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage_error")
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
		nil, fmt.Errorf("storage_error")).Once()

	response, err := tsc.client.GetServices(context.Background(), &api_v3.GetServicesRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage_error")
	assert.Nil(t, response)
}

func TestGetOperations(t *testing.T) {
	tsc := newTestServerClient(t)
	tsc.reader.On("GetOperations", matchContext, mock.AnythingOfType("spanstore.OperationQueryParameters")).Return(
		[]spanstore.Operation{
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
	tsc.reader.On("GetOperations", matchContext, mock.AnythingOfType("spanstore.OperationQueryParameters")).Return(
		nil, fmt.Errorf("storage_error")).Once()

	response, err := tsc.client.GetOperations(context.Background(), &api_v3.GetOperationsRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage_error")
	assert.Nil(t, response)
}
