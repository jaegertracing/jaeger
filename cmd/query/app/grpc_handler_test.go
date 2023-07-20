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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/jtracer"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/plugin/metrics/disabled"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
	depsmocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	"github.com/jaegertracing/jaeger/storage/metricsstore"
	metricsmocks "github.com/jaegertracing/jaeger/storage/metricsstore/mocks"
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

	now = time.Now()
)

type grpcServer struct {
	server              *grpc.Server
	lisAddr             net.Addr
	spanReader          *spanstoremocks.Reader
	depReader           *depsmocks.Reader
	metricsQueryService querysvc.MetricsQueryService
	archiveSpanReader   *spanstoremocks.Reader
	archiveSpanWriter   *spanstoremocks.Writer
}

type grpcClient struct {
	api_v2.QueryServiceClient
	metrics.MetricsQueryServiceClient
	conn *grpc.ClientConn
}

func newGRPCServer(t *testing.T, q *querysvc.QueryService, mq querysvc.MetricsQueryService, logger *zap.Logger, tracer *jtracer.JTracer, tenancyMgr *tenancy.Manager) (*grpc.Server, net.Addr) {
	lis, _ := net.Listen("tcp", ":0")
	var grpcOpts []grpc.ServerOption
	if tenancyMgr.Enabled {
		grpcOpts = append(grpcOpts,
			grpc.StreamInterceptor(tenancy.NewGuardingStreamInterceptor(tenancyMgr)),
			grpc.UnaryInterceptor(tenancy.NewGuardingUnaryInterceptor(tenancyMgr)),
		)
	}
	grpcServer := grpc.NewServer(grpcOpts...)
	grpcHandler := NewGRPCHandler(q, mq, GRPCHandlerOptions{
		Logger: logger,
		Tracer: tracer,
		NowFn: func() time.Time {
			return now
		},
	})
	api_v2.RegisterQueryServiceServer(grpcServer, grpcHandler)
	metrics.RegisterMetricsQueryServiceServer(grpcServer, grpcHandler)

	go func() {
		err := grpcServer.Serve(lis)
		require.NoError(t, err)
	}()

	return grpcServer, lis.Addr()
}

func newGRPCClient(t *testing.T, addr string) *grpcClient {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	return &grpcClient{
		QueryServiceClient:        api_v2.NewQueryServiceClient(conn),
		MetricsQueryServiceClient: metrics.NewMetricsQueryServiceClient(conn),
		conn:                      conn,
	}
}

type testOption func(*testQueryService)

type testQueryService struct {
	// metricsQueryService is used when creating a new GRPCHandler.
	metricsQueryService querysvc.MetricsQueryService
}

func withMetricsQuery() testOption {
	reader := &metricsmocks.Reader{}
	return func(ts *testQueryService) {
		ts.metricsQueryService = reader
	}
}

func withServerAndClient(t *testing.T, actualTest func(server *grpcServer, client *grpcClient), options ...testOption) {
	server := initializeTenantedTestServerGRPCWithOptions(t, &tenancy.Manager{}, options...)
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

func TestGetTraceEmptyTraceIDFailure_GRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		server.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
			Return(mockTrace, nil).Once()

		res, err := client.GetTrace(context.Background(), &api_v2.GetTraceRequest{
			TraceID: model.TraceID{},
		})

		assert.NoError(t, err)

		spanResChunk, err := res.Recv()
		assert.ErrorIs(t, err, errUninitializedTraceID)
		assert.Nil(t, spanResChunk)
	})
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

// test from GRPCHandler and not grpcClient as Generated Go client panics with `nil` request
func TestGetTraceNilRequestOnHandlerGRPC(t *testing.T) {
	grpcHandler := &GRPCHandler{}
	err := grpcHandler.GetTrace(nil, nil)
	assert.EqualError(t, err, errNilRequest.Error())
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

func TestArchiveTraceEmptyTraceFailureGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		_, err := client.ArchiveTrace(context.Background(), &api_v2.ArchiveTraceRequest{
			TraceID: model.TraceID{},
		})
		assert.ErrorIs(t, err, errUninitializedTraceID)
	})
}

// test from GRPCHandler and not grpcClient as Generated Go client panics with `nil` request
func TestArchiveTraceNilRequestOnHandlerGRPC(t *testing.T) {
	grpcHandler := &GRPCHandler{}
	_, err := grpcHandler.ArchiveTrace(context.Background(), nil)
	assert.EqualError(t, err, errNilRequest.Error())
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
		assert.NoError(t, err)

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
		assert.NoError(t, err)

		spanResChunk, err := res.Recv()
		assert.NoError(t, err)
		assert.Len(t, spanResChunk.Spans, 10)

		spanResChunk, err = res.Recv()
		assert.NoError(t, err)
		assert.Len(t, spanResChunk.Spans, 1)
	})
}

func TestFindTracesMissingQuery_GRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		res, err := client.FindTraces(context.Background(), &api_v2.FindTracesRequest{
			Query: nil,
		})
		assert.NoError(t, err)

		spanResChunk, err := res.Recv()
		assert.Error(t, err)
		assert.Nil(t, spanResChunk)
	})
}

func TestFindTracesFailure_GRPC(t *testing.T) {
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

// test from GRPCHandler and not grpcClient as Generated Go client panics with `nil` request
func TestFindTracesNilRequestOnHandlerGRPC(t *testing.T) {
	grpcHandler := &GRPCHandler{}
	err := grpcHandler.FindTraces(nil, nil)
	assert.EqualError(t, err, errNilRequest.Error())
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

// test from GRPCHandler and not grpcClient as Generated Go client panics with `nil` request
func TestGetOperationsNilRequestOnHandlerGRPC(t *testing.T) {
	grpcHandler := &GRPCHandler{}
	_, err := grpcHandler.GetOperations(context.Background(), nil)
	assert.EqualError(t, err, errNilRequest.Error())
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
		withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
			_, err := client.GetDependencies(context.Background(), &api_v2.GetDependenciesRequest{
				StartTime: input.startTime,
				EndTime:   input.endTime,
			})

			assert.Error(t, err)
		})
	}
}

// test from GRPCHandler and not grpcClient as Generated Go client panics with `nil` request
func TestGetDependenciesNilRequestOnHandlerGRPC(t *testing.T) {
	grpcHandler := &GRPCHandler{}
	_, err := grpcHandler.GetDependencies(context.Background(), nil)
	assert.EqualError(t, err, errNilRequest.Error())
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

func TestGetMetricsSuccessGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		baseQueryParam := &metrics.MetricsQueryBaseRequest{
			ServiceNames: []string{"foo"},
		}
		for _, tc := range []struct {
			mockMethod    string
			mockParamType string
			testFn        func(client *grpcClient) (*metrics.GetMetricsResponse, error)
		}{
			{
				mockMethod:    "GetLatencies",
				mockParamType: "*metricsstore.LatenciesQueryParameters",
				testFn: func(client *grpcClient) (*metrics.GetMetricsResponse, error) {
					return client.GetLatencies(context.Background(), &metrics.GetLatenciesRequest{Quantile: 0.95, BaseRequest: baseQueryParam})
				},
			},
			{
				mockMethod:    "GetCallRates",
				mockParamType: "*metricsstore.CallRateQueryParameters",
				testFn: func(client *grpcClient) (*metrics.GetMetricsResponse, error) {
					return client.GetCallRates(context.Background(), &metrics.GetCallRatesRequest{BaseRequest: baseQueryParam})
				},
			},
			{
				mockMethod:    "GetErrorRates",
				mockParamType: "*metricsstore.ErrorRateQueryParameters",
				testFn: func(client *grpcClient) (*metrics.GetMetricsResponse, error) {
					return client.GetErrorRates(context.Background(), &metrics.GetErrorRatesRequest{BaseRequest: baseQueryParam})
				},
			},
		} {
			t.Run(tc.mockMethod, func(t *testing.T) {
				expectedMetrics := &metrics.MetricFamily{Name: "foo"}
				m := server.metricsQueryService.(*metricsmocks.Reader)
				m.On(tc.mockMethod, mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType(tc.mockParamType)).
					Return(expectedMetrics, nil).Once()

				res, err := tc.testFn(client)
				require.NoError(t, err)
				assert.Equal(t, expectedMetrics, &res.Metrics)
			})
		}
	}, withMetricsQuery())
}

func TestGetMetricsReaderDisabledGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		baseQueryParam := &metrics.MetricsQueryBaseRequest{
			ServiceNames: []string{"foo"},
		}
		for _, tc := range []struct {
			name   string
			testFn func(client *grpcClient) (*metrics.GetMetricsResponse, error)
		}{
			{
				name: "GetLatencies",
				testFn: func(client *grpcClient) (*metrics.GetMetricsResponse, error) {
					return client.GetLatencies(context.Background(), &metrics.GetLatenciesRequest{Quantile: 0.95, BaseRequest: baseQueryParam})
				},
			},
			{
				name: "GetCallRates",
				testFn: func(client *grpcClient) (*metrics.GetMetricsResponse, error) {
					return client.GetCallRates(context.Background(), &metrics.GetCallRatesRequest{BaseRequest: baseQueryParam})
				},
			},
			{
				name: "GetErrorRates",
				testFn: func(client *grpcClient) (*metrics.GetMetricsResponse, error) {
					return client.GetErrorRates(context.Background(), &metrics.GetErrorRatesRequest{BaseRequest: baseQueryParam})
				},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				res, err := tc.testFn(client)
				require.Error(t, err)
				assert.Nil(t, res)

				assertGRPCError(t, err, codes.Unimplemented, "metrics querying is currently disabled")
			})
		}
	})
}

func TestGetMetricsUseDefaultParamsGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		baseQueryParam := &metrics.MetricsQueryBaseRequest{
			ServiceNames: []string{"foo"},
		}
		request := &metrics.GetCallRatesRequest{
			BaseRequest: baseQueryParam,
		}
		expectedMetrics := &metrics.MetricFamily{Name: "foo"}
		expectedParams := &metricsstore.CallRateQueryParameters{
			BaseQueryParameters: metricsstore.BaseQueryParameters{
				ServiceNames: []string{"foo"},
				EndTime:      &now,
				Lookback:     &defaultMetricsQueryLookbackDuration,
				Step:         &defaultMetricsQueryStepDuration,
				RatePer:      &defaultMetricsQueryRateDuration,
				SpanKinds:    defaultMetricsSpanKinds,
			},
		}
		m := server.metricsQueryService.(*metricsmocks.Reader)
		m.On("GetCallRates", mock.AnythingOfType("*context.valueCtx"), expectedParams).
			Return(expectedMetrics, nil).Once()

		res, err := client.GetCallRates(context.Background(), request)
		require.NoError(t, err)
		assert.Equal(t, expectedMetrics, &res.Metrics)
	}, withMetricsQuery())
}

func TestGetMetricsOverrideDefaultParamsGRPC(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	endTime := time.Now().In(loc)
	lookback := time.Minute
	step := time.Second
	ratePer := time.Hour
	spanKinds := []metrics.SpanKind{metrics.SpanKind_SPAN_KIND_CONSUMER}
	expectedSpanKinds := []string{metrics.SpanKind_SPAN_KIND_CONSUMER.String()}
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		baseQueryParam := &metrics.MetricsQueryBaseRequest{
			ServiceNames: []string{"foo"},
			EndTime:      &endTime,
			Lookback:     &lookback,
			Step:         &step,
			RatePer:      &ratePer,
			SpanKinds:    spanKinds,
		}
		request := &metrics.GetCallRatesRequest{
			BaseRequest: baseQueryParam,
		}
		expectedMetrics := &metrics.MetricFamily{Name: "foo"}
		expectedParams := &metricsstore.CallRateQueryParameters{
			BaseQueryParameters: metricsstore.BaseQueryParameters{
				ServiceNames: baseQueryParam.ServiceNames,
				EndTime:      &endTime,
				Lookback:     &lookback,
				Step:         &step,
				RatePer:      &ratePer,
				SpanKinds:    expectedSpanKinds,
			},
		}
		m := server.metricsQueryService.(*metricsmocks.Reader)
		m.On("GetCallRates", mock.AnythingOfType("*context.valueCtx"), expectedParams).
			Return(expectedMetrics, nil).Once()

		res, err := client.GetCallRates(context.Background(), request)
		require.NoError(t, err)
		assert.Equal(t, expectedMetrics, &res.Metrics)
	}, withMetricsQuery())
}

func TestGetMetricsFailureGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		baseQueryParam := &metrics.MetricsQueryBaseRequest{
			ServiceNames: []string{"foo"},
		}
		for _, tc := range []struct {
			mockMethod    string
			mockParamType string
			testFn        func(client *grpcClient) (*metrics.GetMetricsResponse, error)
			wantErr       string
		}{
			{
				mockMethod:    "GetLatencies",
				mockParamType: "*metricsstore.LatenciesQueryParameters",
				testFn: func(client *grpcClient) (*metrics.GetMetricsResponse, error) {
					return client.GetLatencies(context.Background(), &metrics.GetLatenciesRequest{Quantile: 0.95, BaseRequest: baseQueryParam})
				},
				wantErr: "failed to fetch latencies: storage error",
			},
			{
				mockMethod:    "GetCallRates",
				mockParamType: "*metricsstore.CallRateQueryParameters",
				testFn: func(client *grpcClient) (*metrics.GetMetricsResponse, error) {
					return client.GetCallRates(context.Background(), &metrics.GetCallRatesRequest{BaseRequest: baseQueryParam})
				},
				wantErr: "failed to fetch call rates: storage error",
			},
			{
				mockMethod:    "GetErrorRates",
				mockParamType: "*metricsstore.ErrorRateQueryParameters",
				testFn: func(client *grpcClient) (*metrics.GetMetricsResponse, error) {
					return client.GetErrorRates(context.Background(), &metrics.GetErrorRatesRequest{BaseRequest: baseQueryParam})
				},
				wantErr: "failed to fetch error rates: storage error",
			},
		} {
			t.Run(tc.mockMethod, func(t *testing.T) {
				m := server.metricsQueryService.(*metricsmocks.Reader)
				m.On(tc.mockMethod, mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType(tc.mockParamType)).
					Return(nil, errStorageGRPC).Once()

				res, err := tc.testFn(client)
				require.Nil(t, res)
				require.Error(t, err)

				assertGRPCError(t, err, codes.Internal, tc.wantErr)
			})
		}
	}, withMetricsQuery())
}

func TestGetMinStepDurationSuccessGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		m := server.metricsQueryService.(*metricsmocks.Reader)
		m.On("GetMinStepDuration", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*metricsstore.MinStepDurationQueryParameters")).
			Return(time.Hour, nil).Once()

		res, err := client.GetMinStepDuration(context.Background(), &metrics.GetMinStepDurationRequest{})
		require.NoError(t, err)
		require.Equal(t, time.Hour, res.MinStep)
	}, withMetricsQuery())
}

func TestGetMinStepDurationFailureGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		m := server.metricsQueryService.(*metricsmocks.Reader)
		m.On("GetMinStepDuration", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*metricsstore.MinStepDurationQueryParameters")).
			Return(time.Duration(0), errStorageGRPC).Once()

		res, err := client.GetMinStepDuration(context.Background(), &metrics.GetMinStepDurationRequest{})
		require.Nil(t, res)
		require.Error(t, err)

		assertGRPCError(t, err, codes.Internal, "failed to fetch min step duration: storage error")
	}, withMetricsQuery())
}

func TestGetMetricsInvalidParametersGRPC(t *testing.T) {
	withServerAndClient(t, func(server *grpcServer, client *grpcClient) {
		for _, tc := range []struct {
			name          string
			mockMethod    string
			mockParamType string
			testFn        func(client *grpcClient) (*metrics.GetMetricsResponse, error)
			wantErr       string
		}{
			{
				name:          "GetLatencies missing service names",
				mockMethod:    "GetLatencies",
				mockParamType: "*metricsstore.LatenciesQueryParameters",
				testFn: func(client *grpcClient) (*metrics.GetMetricsResponse, error) {
					return client.GetLatencies(context.Background(), &metrics.GetLatenciesRequest{Quantile: 0.95})
				},
				wantErr: "please provide at least one service name",
			},
			{
				name:          "GetLatencies missing quantile",
				mockMethod:    "GetLatencies",
				mockParamType: "*metricsstore.LatenciesQueryParameters",
				testFn: func(client *grpcClient) (*metrics.GetMetricsResponse, error) {
					return client.GetLatencies(context.Background(), &metrics.GetLatenciesRequest{
						BaseRequest: &metrics.MetricsQueryBaseRequest{
							ServiceNames: []string{"foo"},
						},
					})
				},
				wantErr: "please provide a quantile between (0, 1]",
			},
			{
				name:          "GetCallRates missing service names",
				mockMethod:    "GetCallRates",
				mockParamType: "*metricsstore.CallRateQueryParameters",
				testFn: func(client *grpcClient) (*metrics.GetMetricsResponse, error) {
					return client.GetCallRates(context.Background(), &metrics.GetCallRatesRequest{}) // Test
				},
				wantErr: "please provide at least one service name",
			},
			{
				name:          "GetErrorRates nil request",
				mockMethod:    "GetErrorRates",
				mockParamType: "*metricsstore.ErrorRateQueryParameters",
				testFn: func(client *grpcClient) (*metrics.GetMetricsResponse, error) {
					return client.GetErrorRates(context.Background(), &metrics.GetErrorRatesRequest{})
				},
				wantErr: "please provide at least one service name",
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				m := server.metricsQueryService.(*metricsmocks.Reader)
				m.On(tc.mockMethod, mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType(tc.mockParamType)).
					Times(0)

				res, err := tc.testFn(client)
				require.Nil(t, res)
				require.Error(t, err)

				assertGRPCError(t, err, codes.InvalidArgument, tc.wantErr)
			})
		}
	}, withMetricsQuery())
}

func TestMetricsQueryNilRequestGRPC(t *testing.T) {
	grpcHandler := &GRPCHandler{}
	bqp, err := grpcHandler.newBaseQueryParameters(nil)
	assert.Empty(t, bqp)
	assert.EqualError(t, err, errNilRequest.Error())
}

func initializeTenantedTestServerGRPCWithOptions(t *testing.T, tm *tenancy.Manager, options ...testOption) *grpcServer {
	archiveSpanReader := &spanstoremocks.Reader{}
	archiveSpanWriter := &spanstoremocks.Writer{}

	spanReader := &spanstoremocks.Reader{}
	dependencyReader := &depsmocks.Reader{}
	disabledReader, err := disabled.NewMetricsReader()
	require.NoError(t, err)

	q := querysvc.NewQueryService(
		spanReader,
		dependencyReader,
		querysvc.QueryServiceOptions{
			ArchiveSpanReader: archiveSpanReader,
			ArchiveSpanWriter: archiveSpanWriter,
		})

	tqs := &testQueryService{
		// Disable metrics query by default.
		metricsQueryService: disabledReader,
	}
	for _, opt := range options {
		opt(tqs)
	}

	logger := zap.NewNop()
	tracer := jtracer.NoOp()

	server, addr := newGRPCServer(t, q, tqs.metricsQueryService, logger, tracer, tm)

	return &grpcServer{
		server:              server,
		lisAddr:             addr,
		spanReader:          spanReader,
		depReader:           dependencyReader,
		metricsQueryService: tqs.metricsQueryService,
		archiveSpanReader:   archiveSpanReader,
		archiveSpanWriter:   archiveSpanWriter,
	}
}

func withTenantedServerAndClient(t *testing.T, tm *tenancy.Manager, actualTest func(server *grpcServer, client *grpcClient), options ...testOption) {
	server := initializeTenantedTestServerGRPCWithOptions(t, tm, options...)
	client := newGRPCClient(t, server.lisAddr.String())
	defer server.server.Stop()
	defer client.conn.Close()

	actualTest(server, client)
}

// withOutgoingMetadata returns a Context with metadata for a server to receive
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
		server.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
			Return(mockTrace, nil).Once()

		// First try without tenancy header
		res, err := client.GetTrace(context.Background(), &api_v2.GetTraceRequest{
			TraceID: mockTraceID,
		})

		require.NoError(t, err, "could not initiate GetTraceRequest")

		spanResChunk, err := res.Recv()
		assertGRPCError(t, err, codes.PermissionDenied, "missing tenant header")
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
		require.Equal(t, len(mockTrace.Spans), len(spanResChunk.Spans))
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
		assertGRPCError(t, err, codes.PermissionDenied, "missing tenant header")

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
		server.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
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
				failureCode:    codes.PermissionDenied,
				failureMessage: "missing tenant header",
			},
			{
				name:           "invalid header",
				tenancyHeader:  "not-the-correct-header",
				tenant:         "mercury",
				wantErr:        true,
				failureCode:    codes.PermissionDenied,
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
					require.Equal(t, len(mockTrace.Spans), len(spanResChunk.Spans))
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
			mockReader.On("GetServices", mock.MatchedBy(func(v interface{}) bool {
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
			mockReader.On("GetTrace", mock.MatchedBy(func(v interface{}) bool {
				ctx, ok := v.(context.Context)
				if !ok {
					return false
				}
				if tenancy.GetTenant(ctx) != tenant {
					return false
				}
				return true
			}), mock.AnythingOfType("model.TraceID")).Return(trace, err).Once()
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
					assert.Contains(t, err.Error(), expected.expectedTraceErr.Error())
				}
			})
		}

		server.spanReader.AssertExpectations(t)
	})
}

func TestNewGRPCHandlerWithEmptyOptions(t *testing.T) {
	disabledReader, err := disabled.NewMetricsReader()
	require.NoError(t, err)

	q := querysvc.NewQueryService(
		&spanstoremocks.Reader{},
		&depsmocks.Reader{},
		querysvc.QueryServiceOptions{})

	handler := NewGRPCHandler(q, disabledReader, GRPCHandlerOptions{})

	assert.NotNil(t, handler.logger)
	assert.NotNil(t, handler.tracer)
	assert.NotNil(t, handler.nowFn)
}
