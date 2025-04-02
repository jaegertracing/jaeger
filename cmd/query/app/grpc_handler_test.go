// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2019 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/internal/jtracer"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	spanstoremocks "github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	depsmocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
	"github.com/jaegertracing/jaeger/internal/tenancy"
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

	now = time.Now()
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

func newGRPCServer(t *testing.T, q *querysvc.QueryService, logger *zap.Logger, tracer *jtracer.JTracer, tenancyMgr *tenancy.Manager) (*grpc.Server, net.Addr) {
	lis, _ := net.Listen("tcp", ":0")
	var grpcOpts []grpc.ServerOption
	if tenancyMgr.Enabled {
		grpcOpts = append(grpcOpts,
			grpc.StreamInterceptor(tenancy.NewGuardingStreamInterceptor(tenancyMgr)),
			grpc.UnaryInterceptor(tenancy.NewGuardingUnaryInterceptor(tenancyMgr)),
		)
	}
	grpcServer := grpc.NewServer(grpcOpts...)
	grpcHandler := NewGRPCHandler(q, GRPCHandlerOptions{
		Logger: logger,
		Tracer: tracer,
		NowFn: func() time.Time {
			return now
		},
	})
	api_v2.RegisterQueryServiceServer(grpcServer, grpcHandler)

	go func() {
		err := grpcServer.Serve(lis)
		assert.NoError(t, err)
	}()

	return grpcServer, lis.Addr()
}

func newGRPCClient(t *testing.T, addr string) *grpcClient {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	return &grpcClient{
		QueryServiceClient: api_v2.NewQueryServiceClient(conn),
		conn:               conn,
	}
}

func withServerAndClient(t *testing.T, actualTest func(server *grpcServer, client *grpcClient)) {
	server := initializeTenantedTestServerGRPC(t, &tenancy.Manager{})
	client := newGRPCClient(t, server.lisAddr.String())
	defer server.server.Stop()
	defer client.conn.Close()

	actualTest(server, client)
}

func TestGetTraceSuccessGRPC(t *testing.T) {
	inputs := []struct {
		expectedQuery spanstore.GetTraceParameters
		request       api_v2.GetTraceRequest
	}{
		{
			spanstore.GetTraceParameters{TraceID: mockTraceID},
			api_v2.GetTraceRequest{TraceID: mockTraceID},
		},
		{
			spanstore.GetTraceParameters{
				TraceID:   mockTraceID,
				StartTime: startTime,
				EndTime:   endTime,
			},
			api_v2.GetTraceRequest{
				TraceID:   mockTraceID,
				StartTime: startTime,
				EndTime:   endTime,
			},
		},
	}

	for _, input := range inputs {
		withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
			server.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), input.expectedQuery).
				Return(mockTrace, nil).Once()

			res, err := client.GetTrace(context.Background(), &input.request)

			spanResChunk, _ := res.Recv()

			require.NoError(t, err)
			assert.Equal(t, spanResChunk.Spans[0].TraceID, mockTraceID)
		})
	}
}

func assertGRPCError(t *testing.T, err error, code codes.Code, msg string) {
	s, ok := status.FromError(err)
	require.True(t, ok, "expecting gRPC status")
	assert.Equal(t, code, s.Code())
	assert.Contains(t, s.Message(), msg)
}

func TestGetTraceEmptyTraceIDFailure_GRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		server.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
			Return(mockTrace, nil).Once()

		res, err := client.GetTrace(context.Background(), &api_v2.GetTraceRequest{
			TraceID: model.TraceID{},
		})

		require.NoError(t, err)

		spanResChunk, err := res.Recv()
		require.ErrorIs(t, err, errUninitializedTraceID)
		assert.Nil(t, spanResChunk)
	})
}

func TestGetTraceDBFailureGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		server.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
			Return(nil, errStorageGRPC).Once()

		res, err := client.GetTrace(context.Background(), &api_v2.GetTraceRequest{
			TraceID: mockTraceID,
		})
		require.NoError(t, err)

		spanResChunk, err := res.Recv()
		assertGRPCError(t, err, codes.Internal, "failed to fetch spans from the backend")
		assert.Nil(t, spanResChunk)
	})
}

func TestGetTraceNotFoundGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		server.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
			Return(nil, spanstore.ErrTraceNotFound).Once()

		server.archiveSpanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
			Return(nil, spanstore.ErrTraceNotFound).Once()

		res, err := client.GetTrace(context.Background(), &api_v2.GetTraceRequest{
			TraceID: mockTraceID,
		})
		require.NoError(t, err)
		spanResChunk, err := res.Recv()
		assertGRPCError(t, err, codes.NotFound, "trace not found")
		assert.Nil(t, spanResChunk)
	})
}

// test from GRPCHandler and not grpcClient as Generated Go client panics with `nil` request
func TestGetTraceNilRequestOnHandlerGRPC(t *testing.T) {
	grpcHandler := &GRPCHandler{}
	err := grpcHandler.GetTrace(nil, nil)
	require.EqualError(t, err, errNilRequest.Error())
}

func TestArchiveTraceSuccessGRPC(t *testing.T) {
	inputs := []struct {
		expectedQuery spanstore.GetTraceParameters
		request       api_v2.ArchiveTraceRequest
	}{
		{
			spanstore.GetTraceParameters{TraceID: mockTraceID},
			api_v2.ArchiveTraceRequest{TraceID: mockTraceID},
		},
		{
			spanstore.GetTraceParameters{
				TraceID:   mockTraceID,
				StartTime: startTime,
				EndTime:   endTime,
			},
			api_v2.ArchiveTraceRequest{
				TraceID:   mockTraceID,
				StartTime: startTime,
				EndTime:   endTime,
			},
		},
	}
	for _, input := range inputs {
		withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
			server.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), input.expectedQuery).
				Return(mockTrace, nil).Once()
			server.archiveSpanWriter.On("WriteSpan", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*model.Span")).
				Return(nil).Times(2)

			_, err := client.ArchiveTrace(context.Background(), &input.request)

			require.NoError(t, err)
		})
	}
}

func TestArchiveTraceNotFoundGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		server.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
			Return(nil, spanstore.ErrTraceNotFound).Once()
		server.archiveSpanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
			Return(nil, spanstore.ErrTraceNotFound).Once()

		_, err := client.ArchiveTrace(context.Background(), &api_v2.ArchiveTraceRequest{
			TraceID: mockTraceID,
		})

		assertGRPCError(t, err, codes.NotFound, "trace not found")
	})
}

func TestArchiveTraceEmptyTraceFailureGRPC(t *testing.T) {
	withServerAndClient(t, func(_ *grpcServer, client *grpcClient) {
		_, err := client.ArchiveTrace(context.Background(), &api_v2.ArchiveTraceRequest{
			TraceID: model.TraceID{},
		})
		require.ErrorIs(t, err, errUninitializedTraceID)
	})
}

// test from GRPCHandler and not grpcClient as Generated Go client panics with `nil` request
func TestArchiveTraceNilRequestOnHandlerGRPC(t *testing.T) {
	grpcHandler := &GRPCHandler{}
	_, err := grpcHandler.ArchiveTrace(context.Background(), nil)
	require.EqualError(t, err, errNilRequest.Error())
}

func TestArchiveTraceFailureGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		server.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
			Return(mockTrace, nil).Once()
		server.archiveSpanWriter.On("WriteSpan", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*model.Span")).
			Return(errStorageGRPC).Times(2)

		_, err := client.ArchiveTrace(context.Background(), &api_v2.ArchiveTraceRequest{
			TraceID: mockTraceID,
		})

		assertGRPCError(t, err, codes.Internal, "failed to archive trace")
	})
}

func TestFindTracesSuccessGRPC(t *testing.T) {
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
		require.NoError(t, err)

		spansArr := make([]model.Span, 0, len(mockTraceGRPC.Spans))
		for _, span := range mockTraceGRPC.Spans {
			spansArr = append(spansArr, *span)
		}
		assert.Equal(t, spansArr, spanResChunk.Spans)
	})
}

func TestFindTracesSuccess_SpanStreamingGRPC(t *testing.T) {
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
		require.NoError(t, err)

		spanResChunk, err := res.Recv()
		require.NoError(t, err)
		assert.Len(t, spanResChunk.Spans, 10)

		spanResChunk, err = res.Recv()
		require.NoError(t, err)
		assert.Len(t, spanResChunk.Spans, 1)
	})
}

func TestFindTracesMissingQuery_GRPC(t *testing.T) {
	withServerAndClient(t, func(_ *grpcServer, client *grpcClient) {
		res, err := client.FindTraces(context.Background(), &api_v2.FindTracesRequest{
			Query: nil,
		})
		require.NoError(t, err)

		spanResChunk, err := res.Recv()
		require.Error(t, err)
		assert.Nil(t, spanResChunk)
	})
}

func TestFindTracesFailure_GRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		mockErrorGRPC := errors.New("whatsamattayou")

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
		require.NoError(t, err)

		spanResChunk, err := res.Recv()
		require.Error(t, err)
		assert.Nil(t, spanResChunk)
	})
}

// test from GRPCHandler and not grpcClient as Generated Go client panics with `nil` request
func TestFindTracesNilRequestOnHandlerGRPC(t *testing.T) {
	grpcHandler := &GRPCHandler{}
	err := grpcHandler.FindTraces(nil, nil)
	require.EqualError(t, err, errNilRequest.Error())
}

func TestGetServicesSuccessGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		expectedServices := []string{"trifle", "bling"}
		server.spanReader.On("GetServices", mock.AnythingOfType("*context.valueCtx")).Return(expectedServices, nil).Once()

		res, err := client.GetServices(context.Background(), &api_v2.GetServicesRequest{})
		require.NoError(t, err)
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
		require.NoError(t, err)
		assert.Len(t, res.Operations, len(expectedOperations))
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

// test from GRPCHandler and not grpcClient as Generated Go client panics with `nil` request
func TestGetOperationsNilRequestOnHandlerGRPC(t *testing.T) {
	grpcHandler := &GRPCHandler{}
	_, err := grpcHandler.GetOperations(context.Background(), nil)
	require.EqualError(t, err, errNilRequest.Error())
}

func TestGetDependenciesSuccessGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		expectedDependencies := []model.DependencyLink{{Parent: "killer", Child: "queen", CallCount: 12}}
		endTs := time.Now().UTC()
		server.depReader.On("GetDependencies",
			mock.Anything, // context.Context
			depstore.QueryParameters{
				StartTime: endTs.Add(-defaultDependencyLookbackDuration),
				EndTime:   endTs,
			},
		).Return(expectedDependencies, nil).Times(1)

		res, err := client.GetDependencies(context.Background(), &api_v2.GetDependenciesRequest{
			StartTime: endTs.Add(time.Duration(-1) * defaultDependencyLookbackDuration),
			EndTime:   endTs,
		})
		require.NoError(t, err)
		assert.Equal(t, expectedDependencies, res.Dependencies)
	})
}

func TestGetDependenciesFailureGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		endTs := time.Now().UTC()
		server.depReader.On("GetDependencies",
			mock.Anything, // context.Context
			depstore.QueryParameters{
				StartTime: endTs.Add(-defaultDependencyLookbackDuration),
				EndTime:   endTs,
			},
		).Return(nil, errStorageGRPC).Times(1)

		_, err := client.GetDependencies(context.Background(), &api_v2.GetDependenciesRequest{
			StartTime: endTs.Add(time.Duration(-1) * defaultDependencyLookbackDuration),
			EndTime:   endTs,
		})

		assertGRPCError(t, err, codes.Internal, "failed to fetch dependencies")
	})
}

func TestGetDependenciesFailureUninitializedTimeGRPC(t *testing.T) {
	timeInputs := []struct {
		startTime time.Time
		endTime   time.Time
	}{
		{time.Time{}, time.Time{}},
		{time.Now(), time.Time{}},
		{time.Time{}, time.Now()},
	}

	for _, input := range timeInputs {
		withServerAndClient(t, func(_ *grpcServer, client *grpcClient) {
			_, err := client.GetDependencies(context.Background(), &api_v2.GetDependenciesRequest{
				StartTime: input.startTime,
				EndTime:   input.endTime,
			})

			require.Error(t, err)
		})
	}
}

// test from GRPCHandler and not grpcClient as Generated Go client panics with `nil` request
func TestGetDependenciesNilRequestOnHandlerGRPC(t *testing.T) {
	grpcHandler := &GRPCHandler{}
	_, err := grpcHandler.GetDependencies(context.Background(), nil)
	require.EqualError(t, err, errNilRequest.Error())
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
	require.EqualError(t, err, expectedErr.Error())
}

func initializeTenantedTestServerGRPC(t *testing.T, tm *tenancy.Manager) *grpcServer {
	archiveSpanReader := &spanstoremocks.Reader{}
	archiveSpanWriter := &spanstoremocks.Writer{}

	spanReader := &spanstoremocks.Reader{}
	dependencyReader := &depsmocks.Reader{}

	q := querysvc.NewQueryService(
		v1adapter.NewTraceReader(spanReader),
		dependencyReader,
		querysvc.QueryServiceOptions{
			ArchiveSpanReader: archiveSpanReader,
			ArchiveSpanWriter: archiveSpanWriter,
		})

	logger := zap.NewNop()
	tracer := jtracer.NoOp()

	server, addr := newGRPCServer(t, q, logger, tracer, tm)

	return &grpcServer{
		server:            server,
		lisAddr:           addr,
		spanReader:        spanReader,
		depReader:         dependencyReader,
		archiveSpanReader: archiveSpanReader,
		archiveSpanWriter: archiveSpanWriter,
	}
}

func withTenantedServerAndClient(t *testing.T, tm *tenancy.Manager, actualTest func(server *grpcServer, client *grpcClient)) {
	server := initializeTenantedTestServerGRPC(t, tm)
	client := newGRPCClient(t, server.lisAddr.String())
	defer server.server.Stop()
	defer client.conn.Close()

	actualTest(server, client)
}

// withOutgoingMetadata returns a Context with metadata for a server to receive
// revive:disable-next-line context-as-argument
func withOutgoingMetadata(t *testing.T, ctx context.Context, headerName, headerValue string) context.Context {
	t.Helper()

	md := metadata.New(map[string]string{headerName: headerValue})
	return metadata.NewOutgoingContext(ctx, md)
}

func TestSearchTenancyGRPC(t *testing.T) {
	tm := tenancy.NewManager(&tenancy.Options{
		Enabled: true,
	})
	withTenantedServerAndClient(t, tm, func(server *grpcServer, client *grpcClient) {
		server.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
			Return(mockTrace, nil).Once()

		// First try without tenancy header
		res, err := client.GetTrace(context.Background(), &api_v2.GetTraceRequest{
			TraceID: mockTraceID,
		})

		require.NoError(t, err, "could not initiate GetTraceRequest")

		spanResChunk, err := res.Recv()
		assertGRPCError(t, err, codes.Unauthenticated, "missing tenant header")
		assert.Nil(t, spanResChunk)

		// Next try with tenancy
		res, err = client.GetTrace(
			withOutgoingMetadata(t, context.Background(), tm.Header, "acme"),
			&api_v2.GetTraceRequest{
				TraceID: mockTraceID,
			})

		spanResChunk, _ = res.Recv()

		require.NoError(t, err, "expecting gRPC to succeed with any tenancy header")
		require.NotNil(t, spanResChunk)
		require.NotNil(t, spanResChunk.Spans)
		require.Len(t, spanResChunk.Spans, len(mockTrace.Spans))
		assert.Equal(t, mockTraceID, spanResChunk.Spans[0].TraceID)
	})
}

func TestServicesTenancyGRPC(t *testing.T) {
	tm := tenancy.NewManager(&tenancy.Options{
		Enabled: true,
	})
	withTenantedServerAndClient(t, tm, func(server *grpcServer, client *grpcClient) {
		expectedServices := []string{"trifle", "bling"}
		server.spanReader.On("GetServices", mock.AnythingOfType("*context.valueCtx")).Return(expectedServices, nil).Once()

		// First try without tenancy header
		_, err := client.GetServices(context.Background(), &api_v2.GetServicesRequest{})
		assertGRPCError(t, err, codes.Unauthenticated, "missing tenant header")

		// Next try with tenancy
		res, err := client.GetServices(withOutgoingMetadata(t, context.Background(), tm.Header, "acme"), &api_v2.GetServicesRequest{})
		require.NoError(t, err, "expecting gRPC to succeed with any tenancy header")
		assert.Equal(t, expectedServices, res.Services)
	})
}

func TestSearchTenancyGRPCExplicitList(t *testing.T) {
	tm := tenancy.NewManager(&tenancy.Options{
		Enabled: true,
		Header:  "non-standard-tenant-header",
		Tenants: []string{"mercury", "venus", "mars"},
	})
	withTenantedServerAndClient(t, tm, func(server *grpcServer, client *grpcClient) {
		server.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
			Return(mockTrace, nil).Once()

		for _, tc := range []struct {
			name           string
			tenancyHeader  string
			tenant         string
			wantErr        bool
			failureCode    codes.Code
			failureMessage string
		}{
			{
				name:           "no header",
				wantErr:        true,
				failureCode:    codes.Unauthenticated,
				failureMessage: "missing tenant header",
			},
			{
				name:           "invalid header",
				tenancyHeader:  "not-the-correct-header",
				tenant:         "mercury",
				wantErr:        true,
				failureCode:    codes.Unauthenticated,
				failureMessage: "missing tenant header",
			},
			{
				name:           "missing tenant",
				tenancyHeader:  tm.Header,
				tenant:         "",
				wantErr:        true,
				failureCode:    codes.PermissionDenied,
				failureMessage: "unknown tenant",
			},
			{
				name:           "invalid tenant",
				tenancyHeader:  tm.Header,
				tenant:         "some-other-tenant-not-in-the-list",
				wantErr:        true,
				failureCode:    codes.PermissionDenied,
				failureMessage: "unknown tenant",
			},
			{
				name:          "valid tenant",
				tenancyHeader: tm.Header,
				tenant:        "venus",
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				ctx := context.Background()
				if tc.tenancyHeader != "" {
					ctx = withOutgoingMetadata(t, context.Background(), tc.tenancyHeader, tc.tenant)
				}
				res, err := client.GetTrace(ctx, &api_v2.GetTraceRequest{
					TraceID: mockTraceID,
				})

				require.NoError(t, err, "could not initiate GetTraceRequest")

				spanResChunk, err := res.Recv()

				if tc.wantErr {
					assertGRPCError(t, err, tc.failureCode, tc.failureMessage)
					assert.Nil(t, spanResChunk)
				} else {
					require.NoError(t, err, "expecting gRPC to succeed")
					require.NotNil(t, spanResChunk)
					require.NotNil(t, spanResChunk.Spans)
					require.Len(t, spanResChunk.Spans, len(mockTrace.Spans))
					assert.Equal(t, mockTraceID, spanResChunk.Spans[0].TraceID)
				}
			})
		}
	})
}

func TestTenancyContextFlowGRPC(t *testing.T) {
	tm := tenancy.NewManager(&tenancy.Options{
		Enabled: true,
	})
	withTenantedServerAndClient(t, tm, func(server *grpcServer, client *grpcClient) {
		// Mock a storage backend with tenant 'acme' and 'megacorp'
		allExpectedResults := map[string]struct {
			expectedServices []string
			expectedTrace    *model.Trace
			expectedTraceErr error
		}{
			"acme":     {[]string{"trifle", "bling"}, mockTrace, nil},
			"megacorp": {[]string{"grapefruit"}, nil, errStorageGRPC},
		}

		addTenantedGetServices := func(mockReader *spanstoremocks.Reader, tenant string, expectedServices []string) {
			mockReader.On("GetServices", mock.MatchedBy(func(v any) bool {
				ctx, ok := v.(context.Context)
				if !ok {
					return false
				}
				if tenancy.GetTenant(ctx) != tenant {
					return false
				}
				return true
			})).Return(expectedServices, nil).Once()
		}
		addTenantedGetTrace := func(mockReader *spanstoremocks.Reader, tenant string, trace *model.Trace, err error) {
			mockReader.On("GetTrace", mock.MatchedBy(func(v any) bool {
				ctx, ok := v.(context.Context)
				if !ok {
					return false
				}
				if tenancy.GetTenant(ctx) != tenant {
					return false
				}
				return true
			}), mock.AnythingOfType("spanstore.GetTraceParameters")).Return(trace, err).Once()
		}

		for tenant, expected := range allExpectedResults {
			addTenantedGetServices(server.spanReader, tenant, expected.expectedServices)
			addTenantedGetTrace(server.spanReader, tenant, expected.expectedTrace, expected.expectedTraceErr)
		}

		for tenant, expected := range allExpectedResults {
			t.Run(tenant, func(t *testing.T) {
				// Test context propagation to Unary method.
				resGetServices, err := client.GetServices(withOutgoingMetadata(t, context.Background(), tm.Header, tenant), &api_v2.GetServicesRequest{})
				require.NoError(t, err, "expecting gRPC to succeed with %q tenancy header", tenant)
				assert.Equal(t, expected.expectedServices, resGetServices.Services)

				// Test context propagation to Streaming method.
				resGetTrace, err := client.GetTrace(withOutgoingMetadata(t, context.Background(), tm.Header, tenant),
					&api_v2.GetTraceRequest{
						TraceID: mockTraceID,
					})
				require.NoError(t, err)
				spanResChunk, err := resGetTrace.Recv()

				if expected.expectedTrace != nil {
					assert.Equal(t, expected.expectedTrace.Spans[0].TraceID, spanResChunk.Spans[0].TraceID)
				}
				if expected.expectedTraceErr != nil {
					assert.ErrorContains(t, err, expected.expectedTraceErr.Error())
				}
			})
		}

		server.spanReader.AssertExpectations(t)
	})
}

func TestNewGRPCHandlerWithEmptyOptions(t *testing.T) {
	q := querysvc.NewQueryService(
		v1adapter.NewTraceReader(&spanstoremocks.Reader{}),
		&depsmocks.Reader{},
		querysvc.QueryServiceOptions{})

	handler := NewGRPCHandler(q, GRPCHandlerOptions{})

	assert.NotNil(t, handler.logger)
	assert.NotNil(t, handler.tracer)
	assert.NotNil(t, handler.nowFn)
}
