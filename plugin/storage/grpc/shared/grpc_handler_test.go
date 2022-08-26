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
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/storage_v1"
	grpcMocks "github.com/jaegertracing/jaeger/proto-gen/storage_v1/mocks"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	dependencyStoreMocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	spanStoreMocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

type mockStoragePlugin struct {
	spanReader    *spanStoreMocks.Reader
	spanWriter    *spanStoreMocks.Writer
	archiveReader *spanStoreMocks.Reader
	archiveWriter *spanStoreMocks.Writer
	depsReader    *dependencyStoreMocks.Reader
	streamWriter  *spanStoreMocks.Writer
}

func (plugin *mockStoragePlugin) ArchiveSpanReader() spanstore.Reader {
	return plugin.archiveReader
}

func (plugin *mockStoragePlugin) ArchiveSpanWriter() spanstore.Writer {
	return plugin.archiveWriter
}

func (plugin *mockStoragePlugin) SpanReader() spanstore.Reader {
	return plugin.spanReader
}

func (plugin *mockStoragePlugin) SpanWriter() spanstore.Writer {
	return plugin.spanWriter
}

func (plugin *mockStoragePlugin) DependencyReader() dependencystore.Reader {
	return plugin.depsReader
}

func (plugin *mockStoragePlugin) StreamingSpanWriter() spanstore.Writer {
	return plugin.streamWriter
}

type grpcServerTest struct {
	server *GRPCHandler
	impl   *mockStoragePlugin
}

func withGRPCServer(fn func(r *grpcServerTest)) {
	spanReader := new(spanStoreMocks.Reader)
	spanWriter := new(spanStoreMocks.Writer)
	archiveReader := new(spanStoreMocks.Reader)
	archiveWriter := new(spanStoreMocks.Writer)
	depReader := new(dependencyStoreMocks.Reader)
	streamWriter := new(spanStoreMocks.Writer)

	impl := &mockStoragePlugin{
		spanReader:    spanReader,
		spanWriter:    spanWriter,
		archiveReader: archiveReader,
		archiveWriter: archiveWriter,
		depsReader:    depReader,
		streamWriter:  streamWriter,
	}

	r := &grpcServerTest{
		server: NewGRPCHandlerWithPlugins(impl, impl, impl),
		impl:   impl,
	}
	fn(r)
}

func TestGRPCServerGetServices(t *testing.T) {
	withGRPCServer(func(r *grpcServerTest) {
		r.impl.spanReader.On("GetServices", mock.Anything).
			Return([]string{"service-a"}, nil)

		s, err := r.server.GetServices(context.Background(), &storage_v1.GetServicesRequest{})
		assert.NoError(t, err)
		assert.Equal(t, &storage_v1.GetServicesResponse{Services: []string{"service-a"}}, s)
	})
}

func TestGRPCServerGetOperations(t *testing.T) {
	withGRPCServer(func(r *grpcServerTest) {
		expOperations := []spanstore.Operation{
			{Name: "operation-a", SpanKind: "client"},
			{Name: "operation-a", SpanKind: "server"},
			{Name: "operation-b", SpanKind: "client"},
		}

		r.impl.spanReader.On("GetOperations",
			mock.Anything,
			spanstore.OperationQueryParameters{ServiceName: "service-a"}).
			Return(expOperations, nil)

		resp, err := r.server.GetOperations(context.Background(), &storage_v1.GetOperationsRequest{
			Service: "service-a",
		})
		assert.NoError(t, err)
		assert.Equal(t, len(expOperations), len(resp.Operations))
		for i, operation := range resp.Operations {
			assert.Equal(t, expOperations[i].Name, operation.Name)
			assert.Equal(t, expOperations[i].SpanKind, operation.SpanKind)
		}
	})
}

func TestGRPCServerGetTrace(t *testing.T) {
	withGRPCServer(func(r *grpcServerTest) {
		traceSteam := new(grpcMocks.SpanReaderPlugin_GetTraceServer)
		traceSteam.On("Context").Return(context.Background())
		traceSteam.On("Send", &storage_v1.SpansResponseChunk{Spans: mockTraceSpans}).
			Return(nil)

		var traceSpans []*model.Span
		for i := range mockTraceSpans {
			traceSpans = append(traceSpans, &mockTraceSpans[i])
		}
		r.impl.spanReader.On("GetTrace", mock.Anything, mockTraceID).
			Return(&model.Trace{Spans: traceSpans}, nil)

		err := r.server.GetTrace(&storage_v1.GetTraceRequest{
			TraceID: mockTraceID,
		}, traceSteam)
		assert.NoError(t, err)
	})
}

func TestGRPCServerGetTrace_NotFound(t *testing.T) {
	withGRPCServer(func(r *grpcServerTest) {
		traceSteam := new(grpcMocks.SpanReaderPlugin_GetTraceServer)
		traceSteam.On("Context").Return(context.Background())

		r.impl.spanReader.On("GetTrace", mock.Anything, mockTraceID).
			Return(nil, spanstore.ErrTraceNotFound)

		err := r.server.GetTrace(&storage_v1.GetTraceRequest{
			TraceID: mockTraceID,
		}, traceSteam)
		assert.Equal(t, codes.NotFound, status.Code(err))
	})
}

func TestGRPCServerFindTraces(t *testing.T) {
	withGRPCServer(func(r *grpcServerTest) {
		traceSteam := new(grpcMocks.SpanReaderPlugin_FindTracesServer)
		traceSteam.On("Context").Return(context.Background())
		traceSteam.On("Send", &storage_v1.SpansResponseChunk{Spans: mockTracesSpans[:2]}).
			Return(nil).Once()
		traceSteam.On("Send", &storage_v1.SpansResponseChunk{Spans: mockTracesSpans[2:]}).
			Return(nil).Once()

		var traces []*model.Trace
		var traceID model.TraceID
		var trace *model.Trace
		for i, span := range mockTracesSpans {
			if span.TraceID != traceID {
				trace = &model.Trace{}
				traceID = span.TraceID
				traces = append(traces, trace)
			}
			trace.Spans = append(trace.Spans, &mockTracesSpans[i])
		}

		r.impl.spanReader.On("FindTraces", mock.Anything, &spanstore.TraceQueryParameters{}).
			Return(traces, nil)

		err := r.server.FindTraces(&storage_v1.FindTracesRequest{
			Query: &storage_v1.TraceQueryParameters{},
		}, traceSteam)
		assert.NoError(t, err)
	})
}

func TestGRPCServerFindTraceIDs(t *testing.T) {
	withGRPCServer(func(r *grpcServerTest) {
		r.impl.spanReader.On("FindTraceIDs", mock.Anything, &spanstore.TraceQueryParameters{}).
			Return([]model.TraceID{mockTraceID, mockTraceID2}, nil)

		s, err := r.server.FindTraceIDs(context.Background(), &storage_v1.FindTraceIDsRequest{
			Query: &storage_v1.TraceQueryParameters{},
		})
		assert.NoError(t, err)
		assert.Equal(t, &storage_v1.FindTraceIDsResponse{TraceIDs: []model.TraceID{mockTraceID, mockTraceID2}}, s)
	})
}

func TestGRPCServerWriteSpan(t *testing.T) {
	withGRPCServer(func(r *grpcServerTest) {
		r.impl.spanWriter.On("WriteSpan", context.Background(), &mockTraceSpans[0]).
			Return(nil)

		s, err := r.server.WriteSpan(context.Background(), &storage_v1.WriteSpanRequest{
			Span: &mockTraceSpans[0],
		})
		assert.NoError(t, err)
		assert.Equal(t, &storage_v1.WriteSpanResponse{}, s)
	})
}

func TestGRPCServerWriteSpanStream(t *testing.T) {
	withGRPCServer(func(r *grpcServerTest) {
		stream := new(grpcMocks.StreamingSpanWriterPlugin_WriteSpanStreamServer)
		stream.On("Recv").Return(&storage_v1.WriteSpanRequest{Span: &mockTraceSpans[0]}, nil).Twice().
			On("Recv").Return(nil, io.EOF).Once()
		stream.On("SendAndClose", &storage_v1.WriteSpanResponse{}).Return(nil)
		stream.On("Context").Return(context.Background())
		r.impl.streamWriter.On("WriteSpan", context.Background(), &mockTraceSpans[0]).
			Return(fmt.Errorf("some error")).Once().
			On("WriteSpan", context.Background(), &mockTraceSpans[0]).
			Return(nil)

		err := r.server.WriteSpanStream(stream)
		assert.Error(t, err)
		err = r.server.WriteSpanStream(stream)
		assert.NoError(t, err)
	})
}

func TestGRPCServerWriteSpanStreamWithGRPCError(t *testing.T) {
	withGRPCServer(func(r *grpcServerTest) {
		stream := new(grpcMocks.StreamingSpanWriterPlugin_WriteSpanStreamServer)
		stream.On("Recv").Return(&storage_v1.WriteSpanRequest{Span: &mockTraceSpans[0]}, nil).Twice().
			On("Recv").Return(nil, context.DeadlineExceeded).Once()
		stream.On("SendAndClose", &storage_v1.WriteSpanResponse{}).Return(nil)
		stream.On("Context").Return(context.Background())
		r.impl.streamWriter.On("WriteSpan", context.Background(), &mockTraceSpans[0]).Return(nil)

		err := r.server.WriteSpanStream(stream)
		assert.ErrorContains(t, err, context.DeadlineExceeded.Error())
	})
}

func TestGRPCServerGetDependencies(t *testing.T) {
	withGRPCServer(func(r *grpcServerTest) {
		lookback := time.Duration(1 * time.Second)
		end := time.Now()
		deps := []model.DependencyLink{
			{
				Source: "source",
				Child:  "child",
			},
		}
		r.impl.depsReader.On("GetDependencies", end, lookback).
			Return(deps, nil)

		s, err := r.server.GetDependencies(context.Background(), &storage_v1.GetDependenciesRequest{
			StartTime: end.Add(-lookback),
			EndTime:   end,
		})
		assert.NoError(t, err)
		assert.Equal(t, &storage_v1.GetDependenciesResponse{Dependencies: deps}, s)
	})
}

func TestGRPCServerGetArchiveTrace(t *testing.T) {
	withGRPCServer(func(r *grpcServerTest) {
		traceSteam := new(grpcMocks.SpanReaderPlugin_GetTraceServer)
		traceSteam.On("Context").Return(context.Background())
		traceSteam.On("Send", &storage_v1.SpansResponseChunk{Spans: mockTraceSpans}).
			Return(nil)

		var traceSpans []*model.Span
		for i := range mockTraceSpans {
			traceSpans = append(traceSpans, &mockTraceSpans[i])
		}
		r.impl.archiveReader.On("GetTrace", mock.Anything, mockTraceID).
			Return(&model.Trace{Spans: traceSpans}, nil)

		err := r.server.GetArchiveTrace(&storage_v1.GetTraceRequest{
			TraceID: mockTraceID,
		}, traceSteam)
		assert.NoError(t, err)
	})
}

func TestGRPCServerGetArchiveTrace_NotFound(t *testing.T) {
	withGRPCServer(func(r *grpcServerTest) {
		traceSteam := new(grpcMocks.SpanReaderPlugin_GetTraceServer)
		traceSteam.On("Context").Return(context.Background())

		r.impl.archiveReader.On("GetTrace", mock.Anything, mockTraceID).
			Return(nil, spanstore.ErrTraceNotFound)

		err := r.server.GetArchiveTrace(&storage_v1.GetTraceRequest{
			TraceID: mockTraceID,
		}, traceSteam)
		assert.Equal(t, codes.NotFound, status.Code(err))
	})
}

func TestGRPCServerGetArchiveTrace_Error(t *testing.T) {
	withGRPCServer(func(r *grpcServerTest) {
		traceSteam := new(grpcMocks.SpanReaderPlugin_GetTraceServer)
		traceSteam.On("Context").Return(context.Background())

		r.impl.archiveReader.On("GetTrace", mock.Anything, mockTraceID).
			Return(nil, fmt.Errorf("some error"))

		err := r.server.GetArchiveTrace(&storage_v1.GetTraceRequest{
			TraceID: mockTraceID,
		}, traceSteam)
		assert.Error(t, err)
	})
}

func TestGRPCServerGetArchiveTrace_NoImpl(t *testing.T) {
	withGRPCServer(func(r *grpcServerTest) {
		r.server.impl.ArchiveSpanReader = func() spanstore.Reader { return nil }
		traceSteam := new(grpcMocks.SpanReaderPlugin_GetTraceServer)

		r.impl.archiveReader.On("GetTrace", mock.Anything, mockTraceID).
			Return(nil, fmt.Errorf("some error"))

		err := r.server.GetArchiveTrace(&storage_v1.GetTraceRequest{
			TraceID: mockTraceID,
		}, traceSteam)
		assert.Equal(t, codes.Unimplemented, status.Code(err))
	})
}

func TestGRPCServerGetArchiveTrace_StreamError(t *testing.T) {
	withGRPCServer(func(r *grpcServerTest) {
		traceSteam := new(grpcMocks.SpanReaderPlugin_GetTraceServer)
		traceSteam.On("Context").Return(context.Background())
		traceSteam.On("Send", &storage_v1.SpansResponseChunk{Spans: mockTraceSpans}).
			Return(fmt.Errorf("some error"))

		var traceSpans []*model.Span
		for i := range mockTraceSpans {
			traceSpans = append(traceSpans, &mockTraceSpans[i])
		}
		r.impl.archiveReader.On("GetTrace", mock.Anything, mockTraceID).
			Return(&model.Trace{Spans: traceSpans}, nil)

		err := r.server.GetArchiveTrace(&storage_v1.GetTraceRequest{
			TraceID: mockTraceID,
		}, traceSteam)
		assert.Error(t, err)
	})
}

func TestGRPCServerWriteArchiveSpan_NoImpl(t *testing.T) {
	withGRPCServer(func(r *grpcServerTest) {
		r.server.impl.ArchiveSpanWriter = func() spanstore.Writer { return nil }

		_, err := r.server.WriteArchiveSpan(context.Background(), &storage_v1.WriteSpanRequest{
			Span: &mockTraceSpans[0],
		})
		assert.Equal(t, codes.Unimplemented, status.Code(err))
	})
}

func TestGRPCServerWriteArchiveSpan(t *testing.T) {
	withGRPCServer(func(r *grpcServerTest) {
		r.impl.archiveWriter.On("WriteSpan", mock.Anything, &mockTraceSpans[0]).
			Return(nil)

		s, err := r.server.WriteArchiveSpan(context.Background(), &storage_v1.WriteSpanRequest{
			Span: &mockTraceSpans[0],
		})
		assert.NoError(t, err)
		assert.Equal(t, &storage_v1.WriteSpanResponse{}, s)
	})
}

func TestGRPCServerWriteArchiveSpan_Error(t *testing.T) {
	withGRPCServer(func(r *grpcServerTest) {
		r.impl.archiveWriter.On("WriteSpan", mock.Anything, &mockTraceSpans[0]).
			Return(fmt.Errorf("some error"))

		_, err := r.server.WriteArchiveSpan(context.Background(), &storage_v1.WriteSpanRequest{
			Span: &mockTraceSpans[0],
		})
		assert.Error(t, err)
	})
}

func TestGRPCServerCapabilities(t *testing.T) {
	withGRPCServer(func(r *grpcServerTest) {
		capabilities, err := r.server.Capabilities(context.Background(), &storage_v1.CapabilitiesRequest{})
		assert.NoError(t, err)
		assert.Equal(t, &storage_v1.CapabilitiesResponse{ArchiveSpanReader: true, ArchiveSpanWriter: true, StreamingSpanWriter: true}, capabilities)
	})
}

func TestGRPCServerCapabilities_NoArchive(t *testing.T) {
	withGRPCServer(func(r *grpcServerTest) {
		r.server.impl.ArchiveSpanReader = func() spanstore.Reader { return nil }
		r.server.impl.ArchiveSpanWriter = func() spanstore.Writer { return nil }

		capabilities, err := r.server.Capabilities(context.Background(), &storage_v1.CapabilitiesRequest{})
		assert.NoError(t, err)
		expected := &storage_v1.CapabilitiesResponse{
			ArchiveSpanReader:   false,
			ArchiveSpanWriter:   false,
			StreamingSpanWriter: true,
		}
		assert.Equal(t, expected, capabilities)
	})
}

func TestGRPCServerCapabilities_NoStreamWriter(t *testing.T) {
	withGRPCServer(func(r *grpcServerTest) {
		r.server.impl.StreamingSpanWriter = func() spanstore.Writer { return nil }

		capabilities, err := r.server.Capabilities(context.Background(), &storage_v1.CapabilitiesRequest{})
		assert.NoError(t, err)
		expected := &storage_v1.CapabilitiesResponse{
			ArchiveSpanReader: true,
			ArchiveSpanWriter: true,
		}
		assert.Equal(t, expected, capabilities)
	})
}

func TestNewGRPCHandlerWithPlugins_Nils(t *testing.T) {
	spanReader := new(spanStoreMocks.Reader)
	spanWriter := new(spanStoreMocks.Writer)
	depReader := new(dependencyStoreMocks.Reader)

	impl := &mockStoragePlugin{
		spanReader: spanReader,
		spanWriter: spanWriter,
		depsReader: depReader,
	}

	handler := NewGRPCHandlerWithPlugins(impl, nil, nil)
	assert.Nil(t, handler.impl.ArchiveSpanReader())
	assert.Nil(t, handler.impl.ArchiveSpanWriter())
	assert.Nil(t, handler.impl.StreamingSpanWriter())
}
