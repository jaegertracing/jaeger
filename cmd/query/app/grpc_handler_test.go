// Copyright (c) 2019 The Jaeger Authors.
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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	depsmocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

var (
	errStorageGRPC = errors.New("storage error")

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

type grpcServer struct {
	server            *grpc.Server
	lisAddr           net.Addr
	spanReader        *spanstoremocks.Reader
	depReader         *depsmocks.Reader
	archiveSpanReader *spanstoremocks.Reader
	archiveSpanWriter *spanstoremocks.Writer
}

type grpcClient struct {
	api_v2.QueryServiceClient
	conn *grpc.ClientConn
}

func newGRPCServer(t *testing.T, q *querysvc.QueryService, logger *zap.Logger, tracer opentracing.Tracer) (*grpc.Server, net.Addr) {
	lis, _ := net.Listen("tcp", ":0")
	grpcServer := grpc.NewServer()
	grpcHandler := NewGRPCHandler(q, logger, tracer)
	api_v2.RegisterQueryServiceServer(grpcServer, grpcHandler)

	go func() {
		err := grpcServer.Serve(lis)
		require.NoError(t, err)
	}()

	return grpcServer, lis.Addr()
}

func newGRPCClient(t *testing.T, addr string) *grpcClient {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, addr, grpc.WithInsecure())
	require.NoError(t, err)

	return &grpcClient{
		QueryServiceClient: api_v2.NewQueryServiceClient(conn),
		conn:               conn,
	}
}

func initializeTestServerGRPCWithOptions(t *testing.T) *grpcServer {

	archiveSpanReader := &spanstoremocks.Reader{}
	archiveSpanWriter := &spanstoremocks.Writer{}

	spanReader := &spanstoremocks.Reader{}
	dependencyReader := &depsmocks.Reader{}

	q := querysvc.NewQueryService(spanReader, dependencyReader,
		querysvc.QueryServiceOptions{
			ArchiveSpanReader: archiveSpanReader,
			ArchiveSpanWriter: archiveSpanWriter,
		})

	logger := zap.NewNop()
	tracer := opentracing.NoopTracer{}

	server, addr := newGRPCServer(t, q, logger, tracer)

	return &grpcServer{
		server:            server,
		lisAddr:           addr,
		spanReader:        spanReader,
		depReader:         dependencyReader,
		archiveSpanReader: archiveSpanReader,
		archiveSpanWriter: archiveSpanWriter,
	}
}

func withServerAndClient(t *testing.T, actualTest func(server *grpcServer, client *grpcClient)) {
	server := initializeTestServerGRPCWithOptions(t)
	client := newGRPCClient(t, server.lisAddr.String())
	defer server.server.Stop()
	defer client.conn.Close()

	actualTest(server, client)
}

func TestGetTraceSuccessGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {

		server.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
			Return(mockTrace, nil).Once()

		res, err := client.GetTrace(context.Background(), &api_v2.GetTraceRequest{
			TraceID: mockTraceID,
		})

		spanResChunk, _ := res.Recv()

		assert.NoError(t, err)
		assert.Equal(t, spanResChunk.Spans[0].TraceID, mockTraceID)

	})
}

func assertGRPCError(t *testing.T, err error, code codes.Code, msg string) {
	s, ok := status.FromError(err)
	require.True(t, ok, "expecting gRPC status")
	assert.Equal(t, code, s.Code())
	assert.Contains(t, s.Message(), msg)
}

func TestGetTraceDBFailureGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {

		server.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
			Return(nil, errStorageGRPC).Once()

		res, err := client.GetTrace(context.Background(), &api_v2.GetTraceRequest{
			TraceID: mockTraceID,
		})
		assert.NoError(t, err)

		spanResChunk, err := res.Recv()
		assertGRPCError(t, err, codes.Internal, "failed to fetch spans from the backend")
		assert.Nil(t, spanResChunk)
	})

}

func TestGetTraceNotFoundGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {

		server.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
			Return(nil, spanstore.ErrTraceNotFound).Once()

		server.archiveSpanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
			Return(nil, spanstore.ErrTraceNotFound).Once()

		res, err := client.GetTrace(context.Background(), &api_v2.GetTraceRequest{
			TraceID: mockTraceID,
		})
		assert.NoError(t, err)
		spanResChunk, err := res.Recv()
		assertGRPCError(t, err, codes.NotFound, "trace not found")
		assert.Nil(t, spanResChunk)
	})
}

func TestArchiveTraceSuccessGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		server.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
			Return(mockTrace, nil).Once()
		server.archiveSpanWriter.On("WriteSpan", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*model.Span")).
			Return(nil).Times(2)

		_, err := client.ArchiveTrace(context.Background(), &api_v2.ArchiveTraceRequest{
			TraceID: mockTraceID,
		})

		assert.NoError(t, err)
	})
}

func TestArchiveTraceNotFoundGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		server.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
			Return(nil, spanstore.ErrTraceNotFound).Once()
		server.archiveSpanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
			Return(nil, spanstore.ErrTraceNotFound).Once()

		_, err := client.ArchiveTrace(context.Background(), &api_v2.ArchiveTraceRequest{
			TraceID: mockTraceID,
		})

		assertGRPCError(t, err, codes.NotFound, "trace not found")
	})
}

func TestArchiveTraceFailureGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {

		server.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
			Return(mockTrace, nil).Once()
		server.archiveSpanWriter.On("WriteSpan", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*model.Span")).
			Return(errStorageGRPC).Times(2)

		_, err := client.ArchiveTrace(context.Background(), &api_v2.ArchiveTraceRequest{
			TraceID: mockTraceID,
		})

		assertGRPCError(t, err, codes.Internal, "failed to archive trace")
	})
}

func TestSearchSuccessGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		server.spanReader.On("FindTraces", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*spanstore.TraceQueryParameters")).
			Return([]*model.Trace{mockTraceGRPC}, nil).Once()

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

		spansArr := make([]model.Span, 0, len(mockTraceGRPC.Spans))
		for _, span := range mockTraceGRPC.Spans {
			spansArr = append(spansArr, *span)
		}
		assert.Equal(t, spansArr, spanResChunk.Spans)

	})
}

func TestSearchSuccess_SpanStreamingGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {

		server.spanReader.On("FindTraces", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*spanstore.TraceQueryParameters")).
			Return([]*model.Trace{mockLargeTraceGRPC}, nil).Once()

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

	})
}

func TestSearchFailure_GRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		mockErrorGRPC := fmt.Errorf("whatsamattayou")

		server.spanReader.On("FindTraces", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*spanstore.TraceQueryParameters")).
			Return(nil, mockErrorGRPC).Once()

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
		assert.Error(t, err)
		assert.Nil(t, spanResChunk)
	})
}

func TestGetServicesSuccessGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		expectedServices := []string{"trifle", "bling"}
		server.spanReader.On("GetServices", mock.AnythingOfType("*context.valueCtx")).Return(expectedServices, nil).Once()

		res, err := client.GetServices(context.Background(), &api_v2.GetServicesRequest{})
		assert.NoError(t, err)
		actualServices := res.Services
		assert.Equal(t, expectedServices, actualServices)
	})
}

func TestGetServicesFailureGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		server.spanReader.On("GetServices", mock.AnythingOfType("*context.valueCtx")).Return(nil, errStorageGRPC).Once()
		_, err := client.GetServices(context.Background(), &api_v2.GetServicesRequest{})

		assertGRPCError(t, err, codes.Internal, "failed to fetch services")
	})
}

func TestGetOperationsSuccessGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {

		expectedOperations := []spanstore.Operation{
			{Name: ""},
			{Name: "get", SpanKind: "server"},
			{Name: "get", SpanKind: "client"},
		}
		expectedNames := []string{"", "get"}
		server.spanReader.On("GetOperations",
			mock.AnythingOfType("*context.valueCtx"),
			spanstore.OperationQueryParameters{ServiceName: "abc/trifle"},
		).Return(expectedOperations, nil).Once()

		res, err := client.GetOperations(context.Background(), &api_v2.GetOperationsRequest{
			Service: "abc/trifle",
		})
		assert.NoError(t, err)
		assert.Equal(t, len(expectedOperations), len(res.Operations))
		for i, actualOp := range res.Operations {
			assert.Equal(t, expectedOperations[i].Name, actualOp.Name)
			assert.Equal(t, expectedOperations[i].SpanKind, actualOp.SpanKind)
		}
		assert.ElementsMatch(t, expectedNames, res.OperationNames)
	})
}

func TestGetOperationsFailureGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		server.spanReader.On("GetOperations",
			mock.AnythingOfType("*context.valueCtx"),
			spanstore.OperationQueryParameters{ServiceName: "trifle"},
		).Return(nil, errStorageGRPC).Once()

		_, err := client.GetOperations(context.Background(), &api_v2.GetOperationsRequest{
			Service: "trifle",
		})

		assertGRPCError(t, err, codes.Internal, "failed to fetch operations")
	})
}

func TestGetDependenciesSuccessGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		expectedDependencies := []model.DependencyLink{{Parent: "killer", Child: "queen", CallCount: 12}}
		endTs := time.Now().UTC()
		server.depReader.On("GetDependencies", endTs.Add(time.Duration(-1)*defaultDependencyLookbackDuration), defaultDependencyLookbackDuration).
			Return(expectedDependencies, nil).Times(1)

		res, err := client.GetDependencies(context.Background(), &api_v2.GetDependenciesRequest{
			StartTime: endTs.Add(time.Duration(-1) * defaultDependencyLookbackDuration),
			EndTime:   endTs,
		})
		assert.NoError(t, err)
		assert.Equal(t, expectedDependencies, res.Dependencies)

	})
}

func TestGetDependenciesFailureGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		endTs := time.Now().UTC()
		server.depReader.On("GetDependencies", endTs.Add(time.Duration(-1)*defaultDependencyLookbackDuration), defaultDependencyLookbackDuration).
			Return(nil, errStorageGRPC).Times(1)

		_, err := client.GetDependencies(context.Background(), &api_v2.GetDependenciesRequest{
			StartTime: endTs.Add(time.Duration(-1) * defaultDependencyLookbackDuration),
			EndTime:   endTs,
		})

		assertGRPCError(t, err, codes.Internal, "failed to fetch dependencies")
	})
}

func TestSendSpanChunksError(t *testing.T) {
	g := &GRPCHandler{
		logger: zap.NewNop(),
	}
	expectedErr := assert.AnError
	err := g.sendSpanChunks([]*model.Span{
		{
			OperationName: "blah",
		},
	}, func(*api_v2.SpansResponseChunk) error {
		return expectedErr
	})
	assert.EqualError(t, err, expectedErr.Error())
}
