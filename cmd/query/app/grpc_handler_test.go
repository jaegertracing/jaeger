// Copyright (c) 2019 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	depsmocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

var (
	grpcServerPort    = ":14251"
	errStorageMsgGRPC = "Storage error"
	errStorageGRPC    = errors.New(errStorageMsgGRPC)

	mockTraceIDgrpc = model.NewTraceID(0, 123456)
	mockTraceGRPC   = &model.Trace{
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
	mockLargeTraceGRPC = &model.Trace{
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
			{
				TraceID: mockTraceID,
				SpanID:  model.NewSpanID(3),
				Process: &model.Process{},
			},
			{
				TraceID: mockTraceID,
				SpanID:  model.NewSpanID(4),
				Process: &model.Process{},
			},
			{
				TraceID: mockTraceID,
				SpanID:  model.NewSpanID(5),
				Process: &model.Process{},
			},
			{
				TraceID: mockTraceID,
				SpanID:  model.NewSpanID(6),
				Process: &model.Process{},
			},
			{
				TraceID: mockTraceID,
				SpanID:  model.NewSpanID(7),
				Process: &model.Process{},
			},
			{
				TraceID: mockTraceID,
				SpanID:  model.NewSpanID(8),
				Process: &model.Process{},
			},
			{
				TraceID: mockTraceID,
				SpanID:  model.NewSpanID(9),
				Process: &model.Process{},
			},
			{
				TraceID: mockTraceID,
				SpanID:  model.NewSpanID(10),
				Process: &model.Process{},
			},
			{
				TraceID: mockTraceID,
				SpanID:  model.NewSpanID(11),
				Process: &model.Process{},
			},
		},
		Warnings: []string{},
	}
)

func newGRPCServer(t *testing.T, q *querysvc.QueryService, logger *zap.Logger, tracer opentracing.Tracer) (*grpc.Server, net.Addr) {
	lis, _ := net.Listen("tcp", grpcServerPort)
	grpcServer := grpc.NewServer()
	grpcHandler := NewGRPCHandler(*q, logger, tracer)
	api_v2.RegisterQueryServiceServer(grpcServer, grpcHandler)

	go func() {
		err := grpcServer.Serve(lis)
		require.NoError(t, err)
	}()

	return grpcServer, lis.Addr()
}

func newGRPCClient(t *testing.T, addr net.Addr) (api_v2.QueryServiceClient, *grpc.ClientConn) {
	conn, err := grpc.Dial(addr.String(), grpc.WithInsecure())
	require.NoError(t, err)
	return api_v2.NewQueryServiceClient(conn), conn
}

func newQueryService(options querysvc.QueryServiceOptions) (*querysvc.QueryService, *spanstoremocks.Reader, *depsmocks.Reader) {
	readStorage := &spanstoremocks.Reader{}
	dependencyStorage := &depsmocks.Reader{}
	return querysvc.NewQueryService(readStorage, dependencyStorage, options), readStorage, dependencyStorage
}

func initializeTestServerGRPC(t *testing.T) (*grpc.Server, net.Addr, *spanstoremocks.Reader, *depsmocks.Reader) {
	q, readStorage, dependencyStorage := newQueryService(querysvc.QueryServiceOptions{})
	logger := zap.NewNop()
	tracer := opentracing.NoopTracer{}

	grpcServer, addr := newGRPCServer(t, q, logger, tracer)

	return grpcServer, addr, readStorage, dependencyStorage
}

func initializeTestServerGRPCWithOptions(t *testing.T) (*grpc.Server, net.Addr, *spanstoremocks.Reader, *depsmocks.Reader, *spanstoremocks.Reader, *spanstoremocks.Writer) {
	archiveSpanReader := &spanstoremocks.Reader{}
	archiveSpanWriter := &spanstoremocks.Writer{}
	q, readStorage, dependencyStorage := newQueryService(querysvc.QueryServiceOptions{
		ArchiveSpanReader: archiveSpanReader,
		ArchiveSpanWriter: archiveSpanWriter,
	})
	logger := zap.NewNop()
	tracer := opentracing.NoopTracer{}

	grpcServer, addr := newGRPCServer(t, q, logger, tracer)

	return grpcServer, addr, readStorage, dependencyStorage, archiveSpanReader, archiveSpanWriter
}

func TestGetTraceSuccessGRPC(t *testing.T) {
	server, addr, readMock, _ := initializeTestServerGRPC(t)
	defer server.Stop()
	readMock.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(mockTrace, nil).Once()

	client, conn := newGRPCClient(t, addr)
	defer conn.Close()

	res, err := client.GetTrace(context.Background(), &api_v2.GetTraceRequest{
		TraceID: mockTraceIDgrpc,
	})
	spanResChunk, _ := res.Recv()

	assert.NoError(t, err)
	assert.Equal(t, spanResChunk.Spans[0].TraceID, mockTraceID)
}

func TestGetTraceDBFailureGRPC(t *testing.T) {
	server, addr, readMock, _ := initializeTestServerGRPC(t)
	defer server.Stop()
	readMock.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(nil, errStorageGRPC).Once()

	client, conn := newGRPCClient(t, addr)
	defer conn.Close()

	res, err := client.GetTrace(context.Background(), &api_v2.GetTraceRequest{
		TraceID: mockTraceIDgrpc,
	})
	spanResChunk, _ := res.Recv()

	assert.NoError(t, err)
	assert.Len(t, spanResChunk.Spans, 0)
}

func TestGetTraceNotFoundGRPC(t *testing.T) {
	server, addr, readMock, _ := initializeTestServerGRPC(t)
	defer server.Stop()
	readMock.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(nil, spanstore.ErrTraceNotFound).Once()

	client, conn := newGRPCClient(t, addr)
	defer conn.Close()

	res, err := client.GetTrace(context.Background(), &api_v2.GetTraceRequest{
		TraceID: mockTraceIDgrpc,
	})
	spanResChunk, _ := res.Recv()

	assert.NoError(t, err)
	assert.Len(t, spanResChunk.Spans, 0)
}

func TestArchiveTraceSuccessGRPC(t *testing.T) {
	server, addr, readMock, _, _, archiveWriteMock := initializeTestServerGRPCWithOptions(t)
	defer server.Stop()
	readMock.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(mockTrace, nil).Once()
	archiveWriteMock.On("WriteSpan", mock.AnythingOfType("*model.Span"), mock.AnythingOfType("model.TraceID")).
		Return(nil).Once()

	client, conn := newGRPCClient(t, addr)
	defer conn.Close()

	_, err := client.ArchiveTrace(context.Background(), &api_v2.ArchiveTraceRequest{
		TraceID: mockTraceIDgrpc,
	})

	assert.NoError(t, err)
}

func TestArchiveTraceNotFoundGRPC(t *testing.T) {
	server, addr, readMock, _, archiveReadMock, _ := initializeTestServerGRPCWithOptions(t)
	defer server.Stop()
	readMock.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(nil, spanstore.ErrTraceNotFound).Once()
	archiveReadMock.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(nil, spanstore.ErrTraceNotFound).Once()

	client, conn := newGRPCClient(t, addr)
	defer conn.Close()

	_, err := client.ArchiveTrace(context.Background(), &api_v2.ArchiveTraceRequest{
		TraceID: mockTraceIDgrpc,
	})

	assert.Equal(t, err, spanstore.ErrTraceNotFound)
}

func TestArchiveTraceFailureGRPC(t *testing.T) {
	server, addr, readMock, _, _, archiveWriteMock := initializeTestServerGRPCWithOptions(t)
	defer server.Stop()
	readMock.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(mockTrace, nil).Once()
	archiveWriteMock.On("WriteSpan", mock.AnythingOfType("*model.Span"), mock.AnythingOfType("model.TraceID")).
		Return(errStorageGRPC).Once()

	client, conn := newGRPCClient(t, addr)
	defer conn.Close()

	_, err := client.ArchiveTrace(context.Background(), &api_v2.ArchiveTraceRequest{
		TraceID: mockTraceIDgrpc,
	})

	assert.Equal(t, err, errStorageGRPC)
}

func TestSearchSuccessGRPC(t *testing.T) {
	server, addr, readMock, _ := initializeTestServerGRPC(t)
	defer server.Stop()
	readMock.On("FindTraces", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*spanstore.TraceQueryParameters")).
		Return([]*model.Trace{mockTraceGRPC}, nil).Once()

	client, conn := newGRPCClient(t, addr)
	defer conn.Close()

	// Trace query parameters.
	queryParams := &api_v2.TraceQueryParameters{
		ServiceName:   "service",
		OperationName: "operation",
		StartTimeMin:  time.Now().Add(time.Duration(-10) * time.Minute),
		StartTimeMax:  time.Now(),
	}
	res, err := client.FindTraces(context.Background(), &api_v2.FindTracesRequest{
		Query: queryParams,
	})

	spanResChunk, _ := res.Recv()
	assert.NoError(t, err)
	assert.Equal(t, mockTraceGRPC.Spans, spanResChunk.Spans)
}

func TestSearchSuccess_SpanStreamingGRPC(t *testing.T) {
	server, addr, readMock, _ := initializeTestServerGRPC(t)
	defer server.Stop()
	readMock.On("FindTraces", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*spanstore.TraceQueryParameters")).
		Return([]*model.Trace{mockLargeTraceGRPC}, nil).Once()

	client, conn := newGRPCClient(t, addr)
	defer conn.Close()

	// Trace query parameters.
	queryParams := &api_v2.TraceQueryParameters{
		ServiceName:   "service",
		OperationName: "operation",
		StartTimeMin:  time.Now().Add(time.Duration(-10) * time.Minute),
		StartTimeMax:  time.Now(),
	}
	res, err := client.FindTraces(context.Background(), &api_v2.FindTracesRequest{
		Query: queryParams,
	})
	assert.NoError(t, err)

	spanResChunk, err := res.Recv()
	assert.NoError(t, err)
	assert.Len(t, spanResChunk.Spans, 10)

	spanResChunk, err = res.Recv()
	assert.NoError(t, err)
	assert.Len(t, spanResChunk.Spans, 1)
}

func TestSearchFailure_GRPC(t *testing.T) {
	server, addr, readMock, _ := initializeTestServerGRPC(t)
	defer server.Stop()
	mockErrorGRPC := fmt.Errorf("whatsamattayou")

	readMock.On("FindTraces", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*spanstore.TraceQueryParameters")).
		Return(nil, mockErrorGRPC).Once()

	client, conn := newGRPCClient(t, addr)
	defer conn.Close()

	// Trace query parameters.
	queryParams := &api_v2.TraceQueryParameters{
		ServiceName:   "service",
		OperationName: "operation",
		StartTimeMin:  time.Now().Add(time.Duration(-10) * time.Minute),
		StartTimeMax:  time.Now(),
	}
	_, err := client.FindTraces(context.Background(), &api_v2.FindTracesRequest{
		Query: queryParams,
	})

	assert.Equal(t, err, mockErrorGRPC)
}

func TestGetServicesSuccessGRPC(t *testing.T) {
	server, addr, readMock, _ := initializeTestServerGRPC(t)
	defer server.Stop()
	expectedServices := []string{"trifle", "bling"}
	readMock.On("GetServices", mock.AnythingOfType("*context.valueCtx")).Return(expectedServices, nil).Once()

	client, conn := newGRPCClient(t, addr)
	defer conn.Close()

	res, err := client.GetServices(context.Background(), &api_v2.GetServicesRequest{})
	assert.NoError(t, err)
	actualServices := res.Services
	assert.Equal(t, expectedServices, actualServices)
}

func TestGetServicesFailureGRPC(t *testing.T) {
	server, addr, readMock, _ := initializeTestServerGRPC(t)
	defer server.Stop()
	readMock.On("GetServices", mock.AnythingOfType("*context.valueCtx")).Return(nil, errStorageGRPC).Once()

	client, conn := newGRPCClient(t, addr)
	defer conn.Close()

	_, err := client.GetServices(context.Background(), &api_v2.GetServicesRequest{})
	assert.Equal(t, err, errStorageGRPC)
}

func TestGetOperationsSuccessGRPC(t *testing.T) {
	server, addr, readMock, _ := initializeTestServerGRPC(t)
	defer server.Stop()
	expectedOperations := []string{"", "get"}
	readMock.On("GetOperations", mock.AnythingOfType("*context.valueCtx"), "abc/trifle").Return(expectedOperations, nil).Once()

	client, conn := newGRPCClient(t, addr)
	defer conn.Close()

	res, err := client.GetOperations(context.Background(), &api_v2.GetOperationsRequest{
		Service: "abc/trifle",
	})
	assert.NoError(t, err)
	assert.Equal(t, expectedOperations, res.Operations)
}

func TestGetOperationsFailureGRPC(t *testing.T) {
	server, addr, readMock, _ := initializeTestServerGRPC(t)
	defer server.Stop()
	readMock.On("GetOperations", mock.AnythingOfType("*context.valueCtx"), "trifle").Return(nil, errStorageGRPC).Once()

	client, conn := newGRPCClient(t, addr)
	defer conn.Close()

	_, err := client.GetOperations(context.Background(), &api_v2.GetOperationsRequest{
		Service: "trifle",
	})
	assert.Error(t, err)
}

func TestGetDependenciesSuccessGRPC(t *testing.T) {
	server, addr, _, depsmocks := initializeTestServerGRPC(t)
	defer server.Stop()
	expectedDependencies := []model.DependencyLink{{Parent: "killer", Child: "queen", CallCount: 12}}
	endTs := time.Unix(0, 1476374248550*millisToNanosMultiplier)
	depsmocks.On("GetDependencies", endTs, defaultDependencyLookbackDuration).Return(expectedDependencies, nil).Times(1)

	client, conn := newGRPCClient(t, addr)
	defer conn.Close()

	res, err := client.GetDependencies(context.Background(), &api_v2.GetDependenciesRequest{
		StartTime: time.Now().Add(time.Duration(-10) * time.Minute),
		EndTime:   time.Now(),
	})
	assert.NoError(t, err)
	assert.Equal(t, expectedDependencies, res.Dependencies)
}

func TestGetDependenciesFailureGRPC(t *testing.T) {
	server, addr, _, depsmocks := initializeTestServerGRPC(t)
	defer server.Stop()
	endTs := time.Unix(0, 1476374248550*millisToNanosMultiplier)
	depsmocks.On("GetDependencies", endTs, defaultDependencyLookbackDuration).Return(nil, errStorageGRPC).Times(1)

	client, conn := newGRPCClient(t, addr)
	defer conn.Close()

	_, err := client.GetDependencies(context.Background(), &api_v2.GetDependenciesRequest{
		StartTime: time.Now().Add(time.Duration(-10) * time.Minute),
		EndTime:   time.Now(),
	})
	assert.Equal(t, err, errStorageGRPC)
}
