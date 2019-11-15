// Copyright (c) 2019 The Jaeger Authors.
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

package shared

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc/metadata"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/storage_v1"
	grpcMocks "github.com/jaegertracing/jaeger/proto-gen/storage_v1/mocks"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var (
	mockTraceID  = model.NewTraceID(0, 123456)
	mockTraceID2 = model.NewTraceID(0, 123457)

	mockTraceSpans = []model.Span{
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
	}

	mockTracesSpans = []model.Span{
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
			TraceID: mockTraceID2,
			SpanID:  model.NewSpanID(1),
			Process: &model.Process{},
		},
	}
)

type grpcClientTest struct {
	client     *grpcClient
	spanReader *grpcMocks.SpanReaderPluginClient
	spanWriter *grpcMocks.SpanWriterPluginClient
	depsReader *grpcMocks.DependenciesReaderPluginClient
}

func withGRPCClient(fn func(r *grpcClientTest)) {
	spanReader := new(grpcMocks.SpanReaderPluginClient)
	spanWriter := new(grpcMocks.SpanWriterPluginClient)
	depReader := new(grpcMocks.DependenciesReaderPluginClient)

	r := &grpcClientTest{
		client: &grpcClient{
			readerClient:     spanReader,
			writerClient:     spanWriter,
			depsReaderClient: depReader,
		},
		spanReader: spanReader,
		spanWriter: spanWriter,
		depsReader: depReader,
	}
	fn(r)
}

func TestContextUpgradeWithToken(t *testing.T) {
	testBearerToken := "test-bearer-token"
	ctx := spanstore.ContextWithBearerToken(context.Background(), testBearerToken)
	upgradedToken := upgradeContextWithBearerToken(ctx)
	md, ok := metadata.FromOutgoingContext(upgradedToken)
	assert.Truef(t, ok, "Expected metadata in context")
	bearerTokenFromMetadata := md.Get(spanstore.BearerTokenKey)
	assert.Equal(t, []string{testBearerToken}, bearerTokenFromMetadata)
}

func TestContextUpgradeWithoutToken(t *testing.T) {
	upgradedToken := upgradeContextWithBearerToken(context.Background())
	_, ok := metadata.FromOutgoingContext(upgradedToken)
	assert.Falsef(t, ok, "Expected no metadata in context")
}

func TestGRPCClientGetServices(t *testing.T) {
	withGRPCClient(func(r *grpcClientTest) {
		r.spanReader.On("GetServices", mock.Anything, &storage_v1.GetServicesRequest{}).
			Return(&storage_v1.GetServicesResponse{Services: []string{"service-a"}}, nil)

		s, err := r.client.GetServices(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, []string{"service-a"}, s)
	})
}

func TestGRPCClientGetOperations(t *testing.T) {
	withGRPCClient(func(r *grpcClientTest) {
		r.spanReader.On("GetOperations", mock.Anything, &storage_v1.GetOperationsRequest{
			Service: "service-a",
		}).Return(&storage_v1.GetOperationsResponse{
			Operations: []*storage_v1.Operation{{Name: "operation-a", SpanKind: ""}},
		}, nil)

		s, err := r.client.GetOperations(context.Background(), &spanstore.OperationQueryParameters{ServiceName: "service-a", SpanKind: ""})
		assert.NoError(t, err)
		assert.Equal(t, []*spanstore.Operation{{Name: "operation-a", SpanKind: ""}}, s)
	})
}

func TestGRPCClientGetTrace(t *testing.T) {
	withGRPCClient(func(r *grpcClientTest) {
		traceClient := new(grpcMocks.SpanReaderPlugin_GetTraceClient)
		traceClient.On("Recv").Return(&storage_v1.SpansResponseChunk{
			Spans: mockTraceSpans,
		}, nil).Once()
		traceClient.On("Recv").Return(nil, io.EOF)
		r.spanReader.On("GetTrace", mock.Anything, &storage_v1.GetTraceRequest{
			TraceID: mockTraceID,
		}).Return(traceClient, nil)

		var expectedSpans []*model.Span
		for i := range mockTraceSpans {
			expectedSpans = append(expectedSpans, &mockTraceSpans[i])
		}

		s, err := r.client.GetTrace(context.Background(), mockTraceID)
		assert.NoError(t, err)
		assert.Equal(t, &model.Trace{
			Spans: expectedSpans,
		}, s)
	})
}

func TestGRPCClientGetTrace_StreamError(t *testing.T) {
	withGRPCClient(func(r *grpcClientTest) {
		traceClient := new(grpcMocks.SpanReaderPlugin_GetTraceClient)
		traceClient.On("Recv").Return(nil, errors.New("an error"))
		r.spanReader.On("GetTrace", mock.Anything, &storage_v1.GetTraceRequest{
			TraceID: mockTraceID,
		}).Return(traceClient, nil)

		s, err := r.client.GetTrace(context.Background(), mockTraceID)
		assert.Error(t, err)
		assert.Nil(t, s)
	})
}

func TestGRPCClientGetTrace_NoTrace(t *testing.T) {
	withGRPCClient(func(r *grpcClientTest) {
		r.spanReader.On("GetTrace", mock.Anything, &storage_v1.GetTraceRequest{
			TraceID: mockTraceID,
		}).Return(nil, spanstore.ErrTraceNotFound)

		s, err := r.client.GetTrace(context.Background(), mockTraceID)
		assert.Error(t, err)
		assert.Nil(t, s)
	})
}

func TestGRPCClientGetTrace_StreamErrorTraceNotFound(t *testing.T) {
	withGRPCClient(func(r *grpcClientTest) {
		traceClient := new(grpcMocks.SpanReaderPlugin_GetTraceClient)
		traceClient.On("Recv").Return(nil, spanstore.ErrTraceNotFound)
		r.spanReader.On("GetTrace", mock.Anything, &storage_v1.GetTraceRequest{
			TraceID: mockTraceID,
		}).Return(traceClient, nil)

		s, err := r.client.GetTrace(context.Background(), mockTraceID)
		assert.Equal(t, spanstore.ErrTraceNotFound, err)
		assert.Nil(t, s)
	})
}

func TestGRPCClientFindTraces(t *testing.T) {
	withGRPCClient(func(r *grpcClientTest) {
		traceClient := new(grpcMocks.SpanReaderPlugin_FindTracesClient)
		traceClient.On("Recv").Return(&storage_v1.SpansResponseChunk{
			Spans: mockTracesSpans,
		}, nil).Once()
		traceClient.On("Recv").Return(nil, io.EOF)
		r.spanReader.On("FindTraces", mock.Anything, &storage_v1.FindTracesRequest{
			Query: &storage_v1.TraceQueryParameters{},
		}).Return(traceClient, nil)

		s, err := r.client.FindTraces(context.Background(), &spanstore.TraceQueryParameters{})
		assert.NoError(t, err)
		assert.NotNil(t, s)
		assert.Equal(t, 2, len(s))
	})
}

func TestGRPCClientFindTraces_Error(t *testing.T) {
	withGRPCClient(func(r *grpcClientTest) {
		r.spanReader.On("FindTraces", mock.Anything, &storage_v1.FindTracesRequest{
			Query: &storage_v1.TraceQueryParameters{},
		}).Return(nil, errors.New("an error"))

		s, err := r.client.FindTraces(context.Background(), &spanstore.TraceQueryParameters{})
		assert.Error(t, err)
		assert.Nil(t, s)
	})
}

func TestGRPCClientFindTraces_RecvError(t *testing.T) {
	withGRPCClient(func(r *grpcClientTest) {
		traceClient := new(grpcMocks.SpanReaderPlugin_FindTracesClient)
		traceClient.On("Recv").Return(nil, errors.New("an error"))
		r.spanReader.On("FindTraces", mock.Anything, &storage_v1.FindTracesRequest{
			Query: &storage_v1.TraceQueryParameters{},
		}).Return(traceClient, nil)

		s, err := r.client.FindTraces(context.Background(), &spanstore.TraceQueryParameters{})
		assert.Error(t, err)
		assert.Nil(t, s)
	})
}

func TestGRPCClientFindTraceIDs(t *testing.T) {
	withGRPCClient(func(r *grpcClientTest) {
		r.spanReader.On("FindTraceIDs", mock.Anything, &storage_v1.FindTraceIDsRequest{
			Query: &storage_v1.TraceQueryParameters{},
		}).Return(&storage_v1.FindTraceIDsResponse{
			TraceIDs: []model.TraceID{mockTraceID, mockTraceID2},
		}, nil)

		s, err := r.client.FindTraceIDs(context.Background(), &spanstore.TraceQueryParameters{})
		assert.NoError(t, err)
		assert.Equal(t, []model.TraceID{mockTraceID, mockTraceID2}, s)
	})
}

func TestGRPCClientWriteSpan(t *testing.T) {
	withGRPCClient(func(r *grpcClientTest) {
		r.spanWriter.On("WriteSpan", mock.Anything, &storage_v1.WriteSpanRequest{
			Span: &mockTraceSpans[0],
		}).Return(&storage_v1.WriteSpanResponse{}, nil)

		err := r.client.WriteSpan(&mockTraceSpans[0])
		assert.NoError(t, err)
	})
}

func TestGRPCClientGetDependencies(t *testing.T) {
	withGRPCClient(func(r *grpcClientTest) {
		lookback := time.Duration(1 * time.Second)
		end := time.Now()
		deps := []model.DependencyLink{
			{
				Source: "source",
				Child:  "child",
			},
		}
		r.depsReader.On("GetDependencies", mock.Anything, &storage_v1.GetDependenciesRequest{
			StartTime: end.Add(-lookback),
			EndTime:   end,
		}).Return(&storage_v1.GetDependenciesResponse{Dependencies: deps}, nil)

		s, err := r.client.GetDependencies(end, lookback)
		assert.NoError(t, err)
		assert.Equal(t, deps, s)
	})
}
