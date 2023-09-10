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

// import (
// 	"math"
// 	"testing"
// 	"time"

// 	"github.com/stretchr/testify/assert"
// 	"github.com/stretchr/testify/require"
// 	semconv "go.opentelemetry.io/collector/semconv/v1.5.0"
// 	"go.opentelemetry.io/otel/trace"

// 	"github.com/jaegertracing/jaeger/model"
// 	commonv1 "github.com/jaegertracing/jaeger/proto-gen/otel/common/v1"
// 	resourcev1 "github.com/jaegertracing/jaeger/proto-gen/otel/resource/v1"
// 	v1 "github.com/jaegertracing/jaeger/proto-gen/otel/trace/v1"
// )

// const (
// 	tagStatusCode = "status.code"
// 	tagStatusMsg  = "status.message"

// 	tagSpanKind      = "span.kind"
// 	tagError         = "error"
// 	tagMessage       = "message"
// 	tagHTTPStatusMsg = "http.status_message"
// 	tagW3CTraceState = "w3c.tracestate"
// )

// var ts = time.Date(2021, 6, 14, 6, 0, 0, 0, time.UTC)

// func TestTranslateSpan(t *testing.T) {
// 	traceID := model.NewTraceID(10, 20)
// 	traceID2 := model.NewTraceID(10, 21)
// 	spanID := model.NewSpanID(30)
// 	spanID2 := model.NewSpanID(999)
// 	spanID3 := model.NewSpanID(888)
// 	spanID4 := model.NewSpanID(8899)
// 	s := &model.Span{
// 		TraceID:       traceID,
// 		SpanID:        spanID,
// 		OperationName: "op_name",
// 		References: []model.SpanRef{
// 			// parent span
// 			{
// 				TraceID: traceID,
// 				SpanID:  spanID2,
// 				RefType: model.SpanRefType_CHILD_OF,
// 			},
// 			{
// 				TraceID: traceID2,
// 				SpanID:  spanID3,
// 				RefType: model.SpanRefType_CHILD_OF,
// 			},
// 			{
// 				TraceID: traceID2,
// 				SpanID:  spanID4,
// 				RefType: model.SpanRefType_FOLLOWS_FROM,
// 			},
// 		},
// 		Flags:     0,
// 		StartTime: ts,
// 		Duration:  15,
// 		Tags: []model.KeyValue{
// 			model.String("k1", "v1"),
// 			model.Bool("k2", true),
// 			model.String(semconv.InstrumentationLibraryName, "servlet"),
// 			model.String(semconv.InstrumentationLibraryVersion, "3.0"),
// 			model.String(tagSpanKind, "client"),
// 			model.Int64(tagStatusCode, 1),
// 			model.String(tagStatusMsg, "msg"),
// 			model.String(tagW3CTraceState, "invalid"),
// 		},
// 		Logs: []model.Log{
// 			{
// 				Timestamp: ts,
// 				Fields: []model.KeyValue{
// 					model.String("k11", "v11"),
// 					model.String("message", "example-event-name"),
// 				},
// 			},
// 		},
// 		Process: &model.Process{
// 			ServiceName: "p1",
// 			Tags: []model.KeyValue{
// 				model.Int64("pv1", 150),
// 				model.String("version", "1.3.4"),
// 			},
// 		},
// 	}

// 	resourceSpans, err := modelToOTLP([]*model.Span{s})
// 	require.NoError(t, err)

// 	assert.Equal(t, []*v1.ResourceSpans{{
// 		Resource: &resourcev1.Resource{
// 			Attributes: []*commonv1.KeyValue{
// 				{Key: "pv1", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 150}}},
// 				{Key: "version", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "1.3.4"}}},
// 				{Key: semconv.AttributeServiceName, Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "p1"}}},
// 			},
// 		},
// 		InstrumentationLibrarySpans: []*v1.InstrumentationLibrarySpans{
// 			{
// 				InstrumentationLibrary: &commonv1.InstrumentationLibrary{
// 					Name:    "servlet",
// 					Version: "3.0",
// 				},
// 				Spans: []*v1.Span{
// 					{
// 						TraceId:      uint64ToTraceID(traceID.High, traceID.Low),
// 						SpanId:       uint64ToSpanID(uint64(spanID)),
// 						ParentSpanId: uint64ToSpanID(uint64(spanID2)),
// 						TraceState:   "invalid",
// 						Name:         "op_name",
// 						Kind:         v1.Span_SPAN_KIND_CLIENT,
// 						Status: &v1.Status{
// 							Code:    v1.Status_STATUS_CODE_OK,
// 							Message: "msg",
// 						},
// 						StartTimeUnixNano: uint64(ts.UnixNano()),
// 						EndTimeUnixNano:   uint64(ts.UnixNano() + 15),
// 						Attributes: []*commonv1.KeyValue{
// 							{Key: "k1", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "v1"}}},
// 							{Key: "k2", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_BoolValue{BoolValue: true}}},
// 						},
// 						Events: []*v1.Span_Event{
// 							{
// 								TimeUnixNano: uint64(ts.UnixNano()),
// 								Name:         "example-event-name",
// 								Attributes: []*commonv1.KeyValue{
// 									{Key: "k11", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "v11"}}},
// 								},
// 							},
// 						},
// 						Links: []*v1.Span_Link{
// 							{
// 								TraceId: uint64ToTraceID(traceID2.High, traceID2.Low),
// 								SpanId:  uint64ToSpanID(uint64(spanID3)),
// 							},
// 							{
// 								TraceId: uint64ToTraceID(traceID2.High, traceID2.Low),
// 								SpanId:  uint64ToSpanID(uint64(spanID4)),
// 							},
// 						},
// 					},
// 				},
// 			},
// 		},
// 	}}, resourceSpans)
// }

// func TestTranslateSpanKind(t *testing.T) {
// 	tests := []struct {
// 		kind         trace.SpanKind
// 		otelSpanKind v1.Span_SpanKind
// 	}{
// 		{
// 			kind:         trace.SpanKindClient,
// 			otelSpanKind: v1.Span_SPAN_KIND_CLIENT,
// 		},
// 		{
// 			kind:         trace.SpanKindServer,
// 			otelSpanKind: v1.Span_SPAN_KIND_SERVER,
// 		},
// 		{
// 			kind:         trace.SpanKindProducer,
// 			otelSpanKind: v1.Span_SPAN_KIND_PRODUCER,
// 		},
// 		{
// 			kind:         trace.SpanKindConsumer,
// 			otelSpanKind: v1.Span_SPAN_KIND_CONSUMER,
// 		},
// 		{
// 			kind:         trace.SpanKindInternal,
// 			otelSpanKind: v1.Span_SPAN_KIND_INTERNAL,
// 		},
// 		{
// 			otelSpanKind: v1.Span_SPAN_KIND_UNSPECIFIED,
// 		},
// 	}
// 	for _, test := range tests {
// 		t.Run(test.kind.String(), func(t *testing.T) {
// 			otelSpanKind := jSpanKindToInternal(test.kind)
// 			assert.Equal(t, test.otelSpanKind, otelSpanKind)
// 		})
// 	}
// }
