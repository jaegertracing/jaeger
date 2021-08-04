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
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/translator/conventions"
	tracetranslator "go.opentelemetry.io/collector/translator/trace"

	"github.com/jaegertracing/jaeger/model"
	commonv1 "github.com/jaegertracing/jaeger/proto-gen/otel/common/v1"
	resourcev1 "github.com/jaegertracing/jaeger/proto-gen/otel/resource/v1"
	v1 "github.com/jaegertracing/jaeger/proto-gen/otel/trace/v1"
)

var ts = time.Date(2021, 6, 14, 6, 0, 0, 0, time.UTC)

func TestTranslateSpan(t *testing.T) {
	traceID := model.NewTraceID(10, 20)
	traceID2 := model.NewTraceID(10, 21)
	spanID := model.NewSpanID(30)
	spanID2 := model.NewSpanID(999)
	spanID3 := model.NewSpanID(888)
	spanID4 := model.NewSpanID(8899)
	s := &model.Span{
		TraceID:       traceID,
		SpanID:        spanID,
		OperationName: "op_name",
		References: []model.SpanRef{
			// parent span
			{
				TraceID: traceID,
				SpanID:  spanID2,
				RefType: model.SpanRefType_CHILD_OF,
			},
			{
				TraceID: traceID2,
				SpanID:  spanID3,
				RefType: model.SpanRefType_CHILD_OF,
			},
			{
				TraceID: traceID2,
				SpanID:  spanID4,
				RefType: model.SpanRefType_FOLLOWS_FROM,
			},
		},
		Flags:     0,
		StartTime: ts,
		Duration:  15,
		Tags: []model.KeyValue{
			model.String("k1", "v1"),
			model.Bool("k2", true),
			model.String(conventions.InstrumentationLibraryName, "servlet"),
			model.String(conventions.InstrumentationLibraryVersion, "3.0"),
			model.String(tracetranslator.TagSpanKind, "client"),
			model.Int64(tracetranslator.TagStatusCode, 1),
			model.String(tracetranslator.TagStatusMsg, "msg"),
			model.String(tracetranslator.TagW3CTraceState, "invalid"),
		},
		Logs: []model.Log{
			{
				Timestamp: ts,
				Fields: []model.KeyValue{
					model.String("k11", "v11"),
					model.String("message", "example-event-name"),
				},
			},
		},
		Process: &model.Process{
			ServiceName: "p1",
			Tags: []model.KeyValue{
				model.Int64("pv1", 150),
				model.String("version", "1.3.4"),
			},
		},
	}

	resourceSpans := jaegerSpansToOTLP([]*model.Span{s})
	assert.Equal(t, []*v1.ResourceSpans{{
		Resource: &resourcev1.Resource{
			Attributes: []*commonv1.KeyValue{
				{Key: "pv1", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 150}}},
				{Key: "version", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "1.3.4"}}},
				{Key: conventions.AttributeServiceName, Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "p1"}}},
			},
		},
		InstrumentationLibrarySpans: []*v1.InstrumentationLibrarySpans{
			{
				InstrumentationLibrary: &commonv1.InstrumentationLibrary{
					Name:    "servlet",
					Version: "3.0",
				},
				Spans: []*v1.Span{
					{
						TraceId:      uint64ToTraceID(traceID.High, traceID.Low),
						SpanId:       uint64ToSpanID(uint64(spanID)),
						ParentSpanId: uint64ToSpanID(uint64(spanID2)),
						TraceState:   "invalid",
						Name:         "op_name",
						Kind:         v1.Span_SPAN_KIND_CLIENT,
						Status: &v1.Status{
							Code:    v1.Status_STATUS_CODE_OK,
							Message: "msg",
						},
						StartTimeUnixNano: uint64(ts.UnixNano()),
						EndTimeUnixNano:   uint64(ts.UnixNano() + 15),
						Attributes: []*commonv1.KeyValue{
							{Key: "k1", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "v1"}}},
							{Key: "k2", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_BoolValue{BoolValue: true}}},
						},
						Events: []*v1.Span_Event{
							{
								TimeUnixNano: uint64(ts.UnixNano()),
								Name:         "example-event-name",
								Attributes: []*commonv1.KeyValue{
									{Key: "k11", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "v11"}}},
								},
							},
						},
						Links: []*v1.Span_Link{
							{
								TraceId: uint64ToTraceID(traceID2.High, traceID2.Low),
								SpanId:  uint64ToSpanID(uint64(spanID3)),
							},
							{
								TraceId: uint64ToTraceID(traceID2.High, traceID2.Low),
								SpanId:  uint64ToSpanID(uint64(spanID4)),
							},
						},
					},
				},
			},
		},
	}}, resourceSpans)
}

func TestTranslateSpanKind(t *testing.T) {
	tests := []struct {
		kind         string
		otelSpanKind v1.Span_SpanKind
	}{
		{
			kind:         "client",
			otelSpanKind: v1.Span_SPAN_KIND_CLIENT,
		},
		{
			kind:         "server",
			otelSpanKind: v1.Span_SPAN_KIND_SERVER,
		},
		{
			kind:         "producer",
			otelSpanKind: v1.Span_SPAN_KIND_PRODUCER,
		},
		{
			kind:         "consumer",
			otelSpanKind: v1.Span_SPAN_KIND_CONSUMER,
		},
		{
			kind:         "internal",
			otelSpanKind: v1.Span_SPAN_KIND_INTERNAL,
		},
		{
			otelSpanKind: v1.Span_SPAN_KIND_UNSPECIFIED,
		},
	}
	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			otelSpanKind := jSpanKindToInternal(test.kind)
			assert.Equal(t, test.otelSpanKind, otelSpanKind)
		})
	}
}

func TestTranslateTProcess_nil(t *testing.T) {
	assert.Nil(t, jProcessToInternalResource(nil))
}

func TestTranslateTags(t *testing.T) {
	tags := []model.KeyValue{
		model.String("str", "str"),
		model.Bool("bool", true),
		model.Int64("int", 150),
		model.Float64("float", 15.6),
		model.Binary("binary", []byte("bytes")),
		model.String("ignore", "str"),
		{
			Key:    "foo",
			VType:  999, // unknown type
			VStr:   "val",
			VInt64: 1,
		},
	}
	otlpKeyValues := jTagsToOTLP(tags, map[string]bool{"ignore": true})
	assert.Equal(t, []*commonv1.KeyValue{
		{
			Key:   "str",
			Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "str"}},
		},
		{
			Key:   "bool",
			Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_BoolValue{BoolValue: true}},
		},
		{
			Key:   "int",
			Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 150}},
		},
		{
			Key:   "float",
			Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_DoubleValue{DoubleValue: 15.6}},
		},
		{
			Key:   "binary",
			Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_BytesValue{BytesValue: []byte("bytes")}},
		},
		{
			Key:   "foo",
			Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "key:\"foo\" v_type:999 v_str:\"val\" v_int64:1 "}},
		},
	}, otlpKeyValues)
}

func TestTranslateSpanStatus(t *testing.T) {
	tests := []struct {
		name       string
		tags       []model.KeyValue
		status     *v1.Status
		ignoreKeys map[string]bool
	}{
		{
			name:       "error tag",
			tags:       []model.KeyValue{model.String(tracetranslator.TagError, "true")},
			status:     &v1.Status{Code: v1.Status_STATUS_CODE_ERROR},
			ignoreKeys: map[string]bool{tracetranslator.TagError: true},
		},
		{
			name:       "status tag int type",
			tags:       []model.KeyValue{model.Int64(tracetranslator.TagStatusCode, 1), model.String(tracetranslator.TagStatusMsg, "foobar")},
			status:     &v1.Status{Message: "foobar", Code: v1.Status_STATUS_CODE_OK},
			ignoreKeys: map[string]bool{tracetranslator.TagStatusCode: true, tracetranslator.TagStatusMsg: true},
		},
		{
			name:       "status tag int type overflow",
			tags:       []model.KeyValue{model.Int64(tracetranslator.TagStatusCode, math.MaxInt64), model.String(tracetranslator.TagStatusMsg, "foobar")},
			status:     &v1.Status{Message: "foobar", Code: v1.Status_STATUS_CODE_UNSET},
			ignoreKeys: map[string]bool{tracetranslator.TagStatusMsg: true},
		},
		{
			name:       "status tag string type",
			tags:       []model.KeyValue{model.String(tracetranslator.TagStatusCode, "1"), model.String(tracetranslator.TagStatusMsg, "foobar")},
			status:     &v1.Status{Message: "foobar", Code: v1.Status_STATUS_CODE_OK},
			ignoreKeys: map[string]bool{tracetranslator.TagStatusCode: true, tracetranslator.TagStatusMsg: true},
		},
		{
			name:       "status tag string type error",
			tags:       []model.KeyValue{model.String(tracetranslator.TagStatusCode, "one"), model.String(tracetranslator.TagStatusMsg, "foobar")},
			status:     &v1.Status{Message: "foobar", Code: v1.Status_STATUS_CODE_UNSET},
			ignoreKeys: map[string]bool{tracetranslator.TagStatusMsg: true},
		},
		{
			name:       "status tag bool type",
			tags:       []model.KeyValue{model.Bool(tracetranslator.TagStatusCode, true), model.String(tracetranslator.TagStatusMsg, "foobar")},
			status:     &v1.Status{Message: "foobar", Code: v1.Status_STATUS_CODE_UNSET},
			ignoreKeys: map[string]bool{tracetranslator.TagStatusMsg: true},
		},
		{
			name:       "HTTP status tag",
			tags:       []model.KeyValue{model.Int64(conventions.AttributeHTTPStatusCode, 200), model.String(tracetranslator.TagHTTPStatusMsg, "all_fine")},
			status:     &v1.Status{Message: "all_fine", Code: v1.Status_STATUS_CODE_UNSET},
			ignoreKeys: map[string]bool{},
		},
		{
			name:       "HTTP status tag error",
			tags:       []model.KeyValue{model.Int64(conventions.AttributeHTTPStatusCode, 500), model.String(tracetranslator.TagHTTPStatusMsg, "some_err")},
			status:     &v1.Status{Message: "some_err", Code: v1.Status_STATUS_CODE_ERROR},
			ignoreKeys: map[string]bool{},
		},
		{
			name:       "HTTP status tag error wrong tag type",
			tags:       []model.KeyValue{model.Bool(conventions.AttributeHTTPStatusCode, true), model.String(tracetranslator.TagHTTPStatusMsg, "some_err")},
			status:     &v1.Status{Code: v1.Status_STATUS_CODE_UNSET},
			ignoreKeys: map[string]bool{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, ignoreKeys := getSpanStatus(test.tags)
			assert.Equal(t, test.status, status)
			assert.Equal(t, test.ignoreKeys, ignoreKeys)

		})
	}
}
