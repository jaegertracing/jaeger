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

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/proto-gen/storage_v1"
	grpcMocks "github.com/jaegertracing/jaeger/proto-gen/storage_v1/mocks"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	dependencyStoreMocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	"github.com/jaegertracing/jaeger/storage/metricsstore"
	metricStoreMocks "github.com/jaegertracing/jaeger/storage/metricsstore/mocks"
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
	metricReader  *metricStoreMocks.Reader
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

func (plugin *mockStoragePlugin) MetricsReader() metricsstore.Reader {
	return plugin.metricReader
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
	metricsReader := new(metricStoreMocks.Reader)

	impl := &mockStoragePlugin{
		spanReader:    spanReader,
		spanWriter:    spanWriter,
		archiveReader: archiveReader,
		archiveWriter: archiveWriter,
		depsReader:    depReader,
		streamWriter:  streamWriter,
		metricReader:  metricsReader,
	}

	r := &grpcServerTest{
		server: NewGRPCHandlerWithPlugins(impl, impl, impl, impl),
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

func TestGRPCServerMetricsReader(t *testing.T) {
	serviceNames := []string{"my_service_name"}
	groupByOperation := true
	endTime := time.Date(2023, time.May, 3, 11, 33, 30, 0, time.UTC)
	lookback := time.Hour
	step := time.Minute
	ratePer := 10 * time.Minute
	spanKinds := []string{"SPAN_KIND_SERVER"}
	quantile := 0.5
	minStep := time.Second

	mf := &metrics.MetricFamily{
		Name: "service_operation_call_rate",
		Type: metrics.MetricType_GAUGE,
		Help: "calls/sec, grouped by service & operation",
		Metrics: []*metrics.Metric{
			{
				Labels: []*metrics.Label{{Name: "service_name", Value: "my_service_name"}, {Name: "operation_name", Value: "my_operation_name"}},
				MetricPoints: []*metrics.MetricPoint{{
					Value:     &metrics.MetricPoint_GaugeValue{GaugeValue: &metrics.GaugeValue{Value: &metrics.GaugeValue_DoubleValue{DoubleValue: 42}}},
					Timestamp: &types.Timestamp{Seconds: 1683147612},
				}},
			},
		},
	}

	t.Run("GetLatencies", func(t *testing.T) {
		withGRPCServer(func(r *grpcServerTest) {
			r.impl.metricReader.On("GetLatencies", mock.Anything, &metricsstore.LatenciesQueryParameters{
				BaseQueryParameters: metricsstore.BaseQueryParameters{
					ServiceNames:     serviceNames,
					GroupByOperation: groupByOperation,
					EndTime:          &endTime,
					Lookback:         &lookback,
					Step:             &step,
					RatePer:          &ratePer,
					SpanKinds:        spanKinds,
				},
				Quantile: quantile,
			}).
				Return(mf, nil)

			s, err := r.server.GetLatencies(context.Background(), &storage_v1.GetLatenciesRequest{
				BaseQueryParameters: &storage_v1.MetricsBaseQueryParameters{
					ServiceNames:     serviceNames,
					GroupByOperation: groupByOperation,
					EndTime:          &types.Timestamp{Seconds: endTime.Unix(), Nanos: int32(endTime.Nanosecond())},
					Lookback:         &types.Duration{Seconds: int64(lookback.Seconds())},
					Step:             &types.Duration{Seconds: int64(step.Seconds())},
					RatePer:          &types.Duration{Seconds: int64(ratePer.Seconds())},
					SpanKinds:        spanKinds,
				},
				Quantile: float32(quantile),
			})
			assert.NoError(t, err)
			assert.Equal(t, &storage_v1.GetLatenciesResponse{MetricFamily: mf}, s)
		})
	})

	t.Run("GetCallRates", func(t *testing.T) {
		withGRPCServer(func(r *grpcServerTest) {
			r.impl.metricReader.On("GetCallRates", mock.Anything, &metricsstore.CallRateQueryParameters{
				BaseQueryParameters: metricsstore.BaseQueryParameters{
					ServiceNames:     serviceNames,
					GroupByOperation: groupByOperation,
					EndTime:          &endTime,
					Lookback:         &lookback,
					Step:             &step,
					RatePer:          &ratePer,
					SpanKinds:        spanKinds,
				},
			}).
				Return(mf, nil)

			s, err := r.server.GetCallRates(context.Background(), &storage_v1.GetCallRatesRequest{
				BaseQueryParameters: &storage_v1.MetricsBaseQueryParameters{
					ServiceNames:     serviceNames,
					GroupByOperation: groupByOperation,
					EndTime:          &types.Timestamp{Seconds: endTime.Unix(), Nanos: int32(endTime.Nanosecond())},
					Lookback:         &types.Duration{Seconds: int64(lookback.Seconds())},
					Step:             &types.Duration{Seconds: int64(step.Seconds())},
					RatePer:          &types.Duration{Seconds: int64(ratePer.Seconds())},
					SpanKinds:        spanKinds,
				},
			})
			assert.NoError(t, err)
			assert.Equal(t, &storage_v1.GetCallRatesResponse{MetricFamily: mf}, s)
		})
	})

	t.Run("GetErrorRates", func(t *testing.T) {
		withGRPCServer(func(r *grpcServerTest) {
			r.impl.metricReader.On("GetErrorRates", mock.Anything, &metricsstore.ErrorRateQueryParameters{
				BaseQueryParameters: metricsstore.BaseQueryParameters{
					ServiceNames:     serviceNames,
					GroupByOperation: groupByOperation,
					EndTime:          &endTime,
					Lookback:         &lookback,
					Step:             &step,
					RatePer:          &ratePer,
					SpanKinds:        spanKinds,
				},
			}).
				Return(mf, nil)

			s, err := r.server.GetErrorRates(context.Background(), &storage_v1.GetErrorRatesRequest{
				BaseQueryParameters: &storage_v1.MetricsBaseQueryParameters{
					ServiceNames:     serviceNames,
					GroupByOperation: groupByOperation,
					EndTime:          &types.Timestamp{Seconds: endTime.Unix(), Nanos: int32(endTime.Nanosecond())},
					Lookback:         &types.Duration{Seconds: int64(lookback.Seconds())},
					Step:             &types.Duration{Seconds: int64(step.Seconds())},
					RatePer:          &types.Duration{Seconds: int64(ratePer.Seconds())},
					SpanKinds:        spanKinds,
				},
			})
			assert.NoError(t, err)
			assert.Equal(t, &storage_v1.GetErrorRatesResponse{MetricFamily: mf}, s)
		})
	})

	t.Run("GetMinStepDuration", func(t *testing.T) {
		withGRPCServer(func(r *grpcServerTest) {
			r.impl.metricReader.On("GetMinStepDuration", mock.Anything, &metricsstore.MinStepDurationQueryParameters{}).
				Return(minStep, nil)

			s, err := r.server.GetMinStepDuration(context.Background(), &storage_v1.GetMinStepDurationRequest{})
			assert.NoError(t, err)
			assert.Equal(t, &storage_v1.GetMinStepDurationResponse{MinStep: &types.Duration{Seconds: int64(minStep.Seconds())}}, s)
		})
	})
}

func TestGRPCServerCapabilities(t *testing.T) {
	withGRPCServer(func(r *grpcServerTest) {
		capabilities, err := r.server.Capabilities(context.Background(), &storage_v1.CapabilitiesRequest{})
		assert.NoError(t, err)
		assert.Equal(t, &storage_v1.CapabilitiesResponse{ArchiveSpanReader: true, ArchiveSpanWriter: true, StreamingSpanWriter: true, MetricsReader: true}, capabilities)
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
			MetricsReader:       true,
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
			MetricsReader:     true,
		}
		assert.Equal(t, expected, capabilities)
	})
}

func TestGRPCServerCapabilities_NoMetricsReader(t *testing.T) {
	withGRPCServer(func(r *grpcServerTest) {
		r.server.impl.MetricsReader = func() metricsstore.Reader { return nil }

		capabilities, err := r.server.Capabilities(context.Background(), &storage_v1.CapabilitiesRequest{})
		assert.NoError(t, err)
		expected := &storage_v1.CapabilitiesResponse{
			ArchiveSpanReader:   true,
			ArchiveSpanWriter:   true,
			StreamingSpanWriter: true,
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

	handler := NewGRPCHandlerWithPlugins(impl, nil, nil, nil)
	assert.Nil(t, handler.impl.ArchiveSpanReader())
	assert.Nil(t, handler.impl.ArchiveSpanWriter())
	assert.Nil(t, handler.impl.StreamingSpanWriter())
	assert.Nil(t, handler.impl.MetricsReader())
}
