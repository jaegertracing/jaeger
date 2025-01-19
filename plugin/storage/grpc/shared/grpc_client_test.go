// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package shared

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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
	client       *GRPCClient
	spanReader   *grpcMocks.SpanReaderPluginClient
	spanWriter   *grpcMocks.SpanWriterPluginClient
	capabilities *grpcMocks.PluginCapabilitiesClient
	depsReader   *grpcMocks.DependenciesReaderPluginClient
	streamWriter *grpcMocks.StreamingSpanWriterPluginClient
}

func withGRPCClient(fn func(r *grpcClientTest)) {
	spanReader := new(grpcMocks.SpanReaderPluginClient)
	spanWriter := new(grpcMocks.SpanWriterPluginClient)
	depReader := new(grpcMocks.DependenciesReaderPluginClient)
	streamWriter := new(grpcMocks.StreamingSpanWriterPluginClient)
	capabilities := new(grpcMocks.PluginCapabilitiesClient)

	r := &grpcClientTest{
		client: &GRPCClient{
			readerClient:       spanReader,
			writerClient:       spanWriter,
			capabilitiesClient: capabilities,
			depsReaderClient:   depReader,
			streamWriterClient: streamWriter,
		},
		spanReader:   spanReader,
		spanWriter:   spanWriter,
		depsReader:   depReader,
		capabilities: capabilities,
		streamWriter: streamWriter,
	}
	fn(r)
}

func TestNewGRPCClient(t *testing.T) {
	conn := &grpc.ClientConn{}
	client := NewGRPCClient(conn, conn)
	assert.NotNil(t, client)

	assert.Implements(t, (*storage_v1.SpanReaderPluginClient)(nil), client.readerClient)
	assert.Implements(t, (*storage_v1.SpanWriterPluginClient)(nil), client.writerClient)
	assert.Implements(t, (*storage_v1.PluginCapabilitiesClient)(nil), client.capabilitiesClient)
	assert.Implements(t, (*storage_v1.DependenciesReaderPluginClient)(nil), client.depsReaderClient)
	assert.Implements(t, (*storage_v1.StreamingSpanWriterPluginClient)(nil), client.streamWriterClient)
}

func TestGRPCClientGetServices(t *testing.T) {
	withGRPCClient(func(r *grpcClientTest) {
		r.spanReader.On("GetServices", mock.Anything, &storage_v1.GetServicesRequest{}).
			Return(&storage_v1.GetServicesResponse{Services: []string{"service-a"}}, nil)

		s, err := r.client.GetServices(context.Background())
		require.NoError(t, err)
		assert.Equal(t, []string{"service-a"}, s)
	})
}

func TestGRPCClientGetOperationsV1(t *testing.T) {
	withGRPCClient(func(r *grpcClientTest) {
		r.spanReader.On("GetOperations", mock.Anything, &storage_v1.GetOperationsRequest{
			Service: "service-a",
		}).Return(&storage_v1.GetOperationsResponse{
			OperationNames: []string{"operation-a"},
		}, nil)

		s, err := r.client.GetOperations(context.Background(),
			spanstore.OperationQueryParameters{ServiceName: "service-a"})
		require.NoError(t, err)
		assert.Equal(t, []spanstore.Operation{{Name: "operation-a"}}, s)
	})
}

func TestGRPCClientGetOperationsV2(t *testing.T) {
	withGRPCClient(func(r *grpcClientTest) {
		r.spanReader.On("GetOperations", mock.Anything, &storage_v1.GetOperationsRequest{
			Service: "service-a",
		}).Return(&storage_v1.GetOperationsResponse{
			Operations: []*storage_v1.Operation{{Name: "operation-a", SpanKind: "server"}},
		}, nil)

		s, err := r.client.GetOperations(context.Background(),
			spanstore.OperationQueryParameters{ServiceName: "service-a"})
		require.NoError(t, err)
		assert.Equal(t, []spanstore.Operation{{Name: "operation-a", SpanKind: "server"}}, s)
	})
}

func TestGRPCClientGetTrace(t *testing.T) {
	withGRPCClient(func(r *grpcClientTest) {
		startTime := time.Date(2020, time.January, 1, 13, 0, 0, 0, time.UTC)
		endTime := time.Date(2020, time.January, 1, 14, 0, 0, 0, time.UTC)
		traceClient := new(grpcMocks.SpanReaderPlugin_GetTraceClient)
		traceClient.On("Recv").Return(&storage_v1.SpansResponseChunk{
			Spans: mockTraceSpans,
		}, nil).Once()
		traceClient.On("Recv").Return(nil, io.EOF)
		r.spanReader.On("GetTrace", mock.Anything, &storage_v1.GetTraceRequest{
			TraceID:   mockTraceID,
			StartTime: startTime,
			EndTime:   endTime,
		}).Return(traceClient, nil)

		var expectedSpans []*model.Span
		for i := range mockTraceSpans {
			expectedSpans = append(expectedSpans, &mockTraceSpans[i])
		}

		s, err := r.client.GetTrace(context.Background(), spanstore.GetTraceParameters{
			TraceID:   mockTraceID,
			StartTime: startTime,
			EndTime:   endTime,
		})
		require.NoError(t, err)
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

		s, err := r.client.GetTrace(context.Background(), spanstore.GetTraceParameters{TraceID: mockTraceID})
		require.Error(t, err)
		assert.Nil(t, s)
	})
}

func TestGRPCClientGetTrace_NoTrace(t *testing.T) {
	withGRPCClient(func(r *grpcClientTest) {
		r.spanReader.On("GetTrace", mock.Anything, &storage_v1.GetTraceRequest{
			TraceID: mockTraceID,
		}).Return(nil, status.Errorf(codes.NotFound, ""))

		s, err := r.client.GetTrace(context.Background(), spanstore.GetTraceParameters{TraceID: mockTraceID})
		assert.Equal(t, spanstore.ErrTraceNotFound, err)
		assert.Nil(t, s)
	})
}

func TestGRPCClientGetTrace_StreamErrorTraceNotFound(t *testing.T) {
	s, _ := status.FromError(spanstore.ErrTraceNotFound)

	withGRPCClient(func(r *grpcClientTest) {
		traceClient := new(grpcMocks.SpanReaderPlugin_GetTraceClient)
		traceClient.On("Recv").Return(nil, s.Err())
		r.spanReader.On("GetTrace", mock.Anything, &storage_v1.GetTraceRequest{
			TraceID: mockTraceID,
		}).Return(traceClient, nil)

		s, err := r.client.GetTrace(context.Background(), spanstore.GetTraceParameters{TraceID: mockTraceID})
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
		require.NoError(t, err)
		assert.NotNil(t, s)
		assert.Len(t, s, 2)
	})
}

func TestGRPCClientFindTraces_Error(t *testing.T) {
	withGRPCClient(func(r *grpcClientTest) {
		r.spanReader.On("FindTraces", mock.Anything, &storage_v1.FindTracesRequest{
			Query: &storage_v1.TraceQueryParameters{},
		}).Return(nil, errors.New("an error"))

		s, err := r.client.FindTraces(context.Background(), &spanstore.TraceQueryParameters{})
		require.Error(t, err)
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
		require.Error(t, err)
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
		require.NoError(t, err)
		assert.Equal(t, []model.TraceID{mockTraceID, mockTraceID2}, s)
	})
}

func TestGRPCClientWriteSpan(t *testing.T) {
	withGRPCClient(func(r *grpcClientTest) {
		r.spanWriter.On("WriteSpan", mock.Anything, &storage_v1.WriteSpanRequest{
			Span: &mockTraceSpans[0],
		}).Return(&storage_v1.WriteSpanResponse{}, nil)

		err := r.client.SpanWriter().WriteSpan(context.Background(), &mockTraceSpans[0])
		require.NoError(t, err)
	})
}

func TestGRPCClientCloseWriter(t *testing.T) {
	withGRPCClient(func(r *grpcClientTest) {
		r.spanWriter.On("Close", mock.Anything, &storage_v1.CloseWriterRequest{}).Return(&storage_v1.CloseWriterResponse{}, nil)

		err := r.client.Close()
		require.NoError(t, err)
	})
}

func TestGRPCClientCloseNotSupported(t *testing.T) {
	withGRPCClient(func(r *grpcClientTest) {
		r.spanWriter.On("Close", mock.Anything, &storage_v1.CloseWriterRequest{}).Return(
			nil, status.Errorf(codes.Unimplemented, "method not implemented"))

		err := r.client.Close()
		require.NoError(t, err)
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

		s, err := r.client.GetDependencies(context.Background(), end, lookback)
		require.NoError(t, err)
		assert.Equal(t, deps, s)
	})
}

func TestGrpcClientStreamWriterWriteSpan(t *testing.T) {
	withGRPCClient(func(r *grpcClientTest) {
		stream := new(grpcMocks.StreamingSpanWriterPlugin_WriteSpanStreamClient)
		r.streamWriter.On("WriteSpanStream", mock.Anything).Return(stream, nil)
		stream.On("Send", &storage_v1.WriteSpanRequest{Span: &mockTraceSpans[0]}).Return(nil)
		err := r.client.StreamingSpanWriter().WriteSpan(context.Background(), &mockTraceSpans[0])
		require.NoError(t, err)
	})
}

func TestGrpcClientCapabilities(t *testing.T) {
	withGRPCClient(func(r *grpcClientTest) {
		r.capabilities.On("Capabilities", mock.Anything, &storage_v1.CapabilitiesRequest{}).
			Return(&storage_v1.CapabilitiesResponse{ArchiveSpanReader: true, ArchiveSpanWriter: true, StreamingSpanWriter: true}, nil)

		capabilities, err := r.client.Capabilities()
		require.NoError(t, err)
		assert.Equal(t, &Capabilities{
			StreamingSpanWriter: true,
		}, capabilities)
	})
}

func TestGrpcClientCapabilities_NotSupported(t *testing.T) {
	withGRPCClient(func(r *grpcClientTest) {
		r.capabilities.On("Capabilities", mock.Anything, &storage_v1.CapabilitiesRequest{}).
			Return(&storage_v1.CapabilitiesResponse{}, nil)

		capabilities, err := r.client.Capabilities()
		require.NoError(t, err)
		assert.Equal(t, &Capabilities{
			StreamingSpanWriter: false,
		}, capabilities)
	})
}

func TestGrpcClientCapabilities_MissingMethod(t *testing.T) {
	withGRPCClient(func(r *grpcClientTest) {
		r.capabilities.On("Capabilities", mock.Anything, &storage_v1.CapabilitiesRequest{}).
			Return(nil, status.Error(codes.Unimplemented, "method not found"))

		capabilities, err := r.client.Capabilities()
		require.NoError(t, err)
		assert.Equal(t, &Capabilities{}, capabilities)
	})
}

func TestGrpcClientArchiveSupported_CommonGrpcError(t *testing.T) {
	withGRPCClient(func(r *grpcClientTest) {
		r.capabilities.On("Capabilities", mock.Anything, &storage_v1.CapabilitiesRequest{}).
			Return(nil, status.Error(codes.Internal, "internal error"))

		_, err := r.client.Capabilities()
		require.Error(t, err)
	})
}
