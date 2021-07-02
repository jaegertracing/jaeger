// Copyright (c) 2021 The Jaeger Authors.
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

package apiv3

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/model"
	_ "github.com/jaegertracing/jaeger/pkg/gogocodec" //force gogo codec registration
	"github.com/jaegertracing/jaeger/proto-gen/api_v3"
	dependencyStoreMocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

func newGrpcServer(t *testing.T, handler *Handler) (*grpc.Server, net.Addr) {
	server := grpc.NewServer()
	api_v3.RegisterQueryServiceServer(server, handler)

	lis, _ := net.Listen("tcp", ":0")
	go func() {
		err := server.Serve(lis)
		require.NoError(t, err)
	}()

	return server, lis.Addr()
}

func TestGetTrace(t *testing.T) {
	r := &spanstoremocks.Reader{}
	r.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).Return(
		&model.Trace{
			Spans: []*model.Span{
				{
					OperationName: "foobar",
				},
			},
		}, nil).Once()

	q := querysvc.NewQueryService(r, &dependencyStoreMocks.Reader{}, querysvc.QueryServiceOptions{})
	h := &Handler{
		QueryService: q,
	}
	server, addr := newGrpcServer(t, h)
	defer server.Stop()

	conn, err := grpc.DialContext(context.Background(), addr.String(), grpc.WithInsecure())
	require.NoError(t, err)
	defer conn.Close()
	client := api_v3.NewQueryServiceClient(conn)
	getTraceStream, err := client.GetTrace(context.Background(), &api_v3.GetTraceRequest{
		TraceId: "156",
	})
	require.NoError(t, err)
	spansChunk, err := getTraceStream.Recv()
	require.NoError(t, err)
	require.Equal(t, 1, len(spansChunk.GetResourceSpans()))
	assert.Equal(t, "foobar", spansChunk.GetResourceSpans()[0].GetInstrumentationLibrarySpans()[0].GetSpans()[0].GetName())
}

func TestGetTrace_storage_error(t *testing.T) {
	r := &spanstoremocks.Reader{}
	r.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).Return(
		nil, fmt.Errorf("storage_error")).Once()

	q := querysvc.NewQueryService(r, &dependencyStoreMocks.Reader{}, querysvc.QueryServiceOptions{})
	h := &Handler{
		QueryService: q,
	}
	server, addr := newGrpcServer(t, h)
	defer server.Stop()

	conn, err := grpc.DialContext(context.Background(), addr.String(), grpc.WithInsecure())
	require.NoError(t, err)
	defer conn.Close()
	client := api_v3.NewQueryServiceClient(conn)
	getTraceStream, err := client.GetTrace(context.Background(), &api_v3.GetTraceRequest{
		TraceId: "156",
	})
	require.NoError(t, err)
	spansChunk, err := getTraceStream.Recv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage_error")
	assert.Nil(t, spansChunk)
}

func TestGetTrace_traceID_error(t *testing.T) {
	r := &spanstoremocks.Reader{}
	r.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).Return(
		&model.Trace{
			Spans: []*model.Span{},
		}, nil).Once()

	q := querysvc.NewQueryService(r, &dependencyStoreMocks.Reader{}, querysvc.QueryServiceOptions{})
	h := &Handler{
		QueryService: q,
	}
	server, addr := newGrpcServer(t, h)
	defer server.Stop()

	conn, err := grpc.DialContext(context.Background(), addr.String(), grpc.WithInsecure())
	require.NoError(t, err)
	defer conn.Close()
	client := api_v3.NewQueryServiceClient(conn)
	getTraceStream, err := client.GetTrace(context.Background(), &api_v3.GetTraceRequest{
		TraceId: "Z",
	})
	require.NoError(t, err)
	spansChunk, err := getTraceStream.Recv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "strconv.ParseUint:")
	assert.Nil(t, spansChunk)
}

func TestFindTraces(t *testing.T) {
	r := &spanstoremocks.Reader{}
	r.On("FindTraces", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*spanstore.TraceQueryParameters")).Return(
		[]*model.Trace{
			{
				Spans: []*model.Span{
					{
						OperationName: "name",
					},
				},
			},
		}, nil).Once()

	q := querysvc.NewQueryService(r, &dependencyStoreMocks.Reader{}, querysvc.QueryServiceOptions{})
	h := &Handler{
		QueryService: q,
	}
	server, addr := newGrpcServer(t, h)
	defer server.Stop()

	conn, err := grpc.DialContext(context.Background(), addr.String(), grpc.WithInsecure())
	require.NoError(t, err)
	defer conn.Close()
	client := api_v3.NewQueryServiceClient(conn)
	responseStream, err := client.FindTraces(context.Background(), &api_v3.FindTracesRequest{
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
	assert.Equal(t, 1, len(recv.GetResourceSpans()))
}

func TestFindTraces_query_nil(t *testing.T) {
	q := querysvc.NewQueryService(&spanstoremocks.Reader{}, &dependencyStoreMocks.Reader{}, querysvc.QueryServiceOptions{})
	h := &Handler{QueryService: q}
	server, addr := newGrpcServer(t, h)
	defer server.Stop()

	conn, err := grpc.DialContext(context.Background(), addr.String(), grpc.WithInsecure())
	require.NoError(t, err)
	defer conn.Close()
	client := api_v3.NewQueryServiceClient(conn)
	responseStream, err := client.FindTraces(context.Background(), &api_v3.FindTracesRequest{})
	require.NoError(t, err)
	recv, err := responseStream.Recv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing query")
	assert.Nil(t, recv)

	responseStream, err = client.FindTraces(context.Background(), &api_v3.FindTracesRequest{
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

func TestFindTraces_storage_error(t *testing.T) {
	r := &spanstoremocks.Reader{}
	r.On("FindTraces", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*spanstore.TraceQueryParameters")).Return(
		nil, fmt.Errorf("storage_error"), nil).Once()

	q := querysvc.NewQueryService(r, &dependencyStoreMocks.Reader{}, querysvc.QueryServiceOptions{})
	h := &Handler{
		QueryService: q,
	}
	server, addr := newGrpcServer(t, h)
	defer server.Stop()

	conn, err := grpc.DialContext(context.Background(), addr.String(), grpc.WithInsecure())
	require.NoError(t, err)
	defer conn.Close()
	client := api_v3.NewQueryServiceClient(conn)
	responseStream, err := client.FindTraces(context.Background(), &api_v3.FindTracesRequest{
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
	r := &spanstoremocks.Reader{}
	r.On("GetServices", mock.AnythingOfType("*context.valueCtx")).Return(
		[]string{"foo"}, nil).Once()

	q := querysvc.NewQueryService(r, &dependencyStoreMocks.Reader{}, querysvc.QueryServiceOptions{})
	h := &Handler{
		QueryService: q,
	}
	server, addr := newGrpcServer(t, h)
	defer server.Stop()

	conn, err := grpc.DialContext(context.Background(), addr.String(), grpc.WithInsecure())
	require.NoError(t, err)
	defer conn.Close()
	client := api_v3.NewQueryServiceClient(conn)
	response, err := client.GetServices(context.Background(), &api_v3.GetServicesRequest{})
	require.NoError(t, err)
	assert.Equal(t, []string{"foo"}, response.GetServices())
}

func TestGetServices_storage_error(t *testing.T) {
	r := &spanstoremocks.Reader{}
	r.On("GetServices", mock.AnythingOfType("*context.valueCtx")).Return(
		nil, fmt.Errorf("storage_error")).Once()

	q := querysvc.NewQueryService(r, &dependencyStoreMocks.Reader{}, querysvc.QueryServiceOptions{})
	h := &Handler{
		QueryService: q,
	}
	server, addr := newGrpcServer(t, h)
	defer server.Stop()

	conn, err := grpc.DialContext(context.Background(), addr.String(), grpc.WithInsecure())
	require.NoError(t, err)
	defer conn.Close()
	client := api_v3.NewQueryServiceClient(conn)
	response, err := client.GetServices(context.Background(), &api_v3.GetServicesRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage_error")
	assert.Nil(t, response)
}

func TestGetOperations(t *testing.T) {
	r := &spanstoremocks.Reader{}
	r.On("GetOperations", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.OperationQueryParameters")).Return(
		[]spanstore.Operation{
			{
				Name: "get_users",
			}}, nil).Once()

	q := querysvc.NewQueryService(r, &dependencyStoreMocks.Reader{}, querysvc.QueryServiceOptions{})
	h := &Handler{
		QueryService: q,
	}
	server, addr := newGrpcServer(t, h)
	defer server.Stop()

	conn, err := grpc.DialContext(context.Background(), addr.String(), grpc.WithInsecure())
	require.NoError(t, err)
	defer conn.Close()
	client := api_v3.NewQueryServiceClient(conn)
	response, err := client.GetOperations(context.Background(), &api_v3.GetOperationsRequest{})
	require.NoError(t, err)
	assert.Equal(t, []*api_v3.Operation{
		{
			Name: "get_users",
		},
	}, response.GetOperations())
}

func TestGetOperations_storage_error(t *testing.T) {
	r := &spanstoremocks.Reader{}
	r.On("GetOperations", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.OperationQueryParameters")).Return(
		nil, fmt.Errorf("storage_error")).Once()

	q := querysvc.NewQueryService(r, &dependencyStoreMocks.Reader{}, querysvc.QueryServiceOptions{})
	h := &Handler{
		QueryService: q,
	}
	server, addr := newGrpcServer(t, h)
	defer server.Stop()

	conn, err := grpc.DialContext(context.Background(), addr.String(), grpc.WithInsecure())
	require.NoError(t, err)
	defer conn.Close()
	client := api_v3.NewQueryServiceClient(conn)
	response, err := client.GetOperations(context.Background(), &api_v3.GetOperationsRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage_error")
	assert.Nil(t, response)
}
