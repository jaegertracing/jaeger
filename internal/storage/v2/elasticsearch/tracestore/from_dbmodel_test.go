// Copyright (c) 2025 The Jaeger Authors.
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Code originally copied from https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/e49500a9b68447cbbe237fa29526ba99e4963f39/pkg/translator/jaeger/jaegerproto_to_traces_test.go

package tracestore

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	conventions "go.opentelemetry.io/collector/semconv/v1.16.0"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
)

var testSpanEventTime = time.Date(2020, 2, 11, 20, 26, 13, 123000, time.UTC)

func TestCodeFromAttr(t *testing.T) {
	tests := []struct {
		name string
		attr pcommon.Value
		code int64
		err  error
	}{
		{
			name: "ok-string",
			attr: pcommon.NewValueStr("0"),
			code: 0,
		},

		{
			name: "ok-int",
			attr: pcommon.NewValueInt(1),
			code: 1,
		},

		{
			name: "wrong-type",
			attr: pcommon.NewValueBool(true),
			code: 0,
			err:  errType,
		},

		{
			name: "invalid-string",
			attr: pcommon.NewValueStr("inf"),
			code: 0,
			err:  strconv.ErrSyntax,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, err := codeFromAttr(test.attr)
			if test.err != nil {
				require.ErrorIs(t, err, test.err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, test.code, code)
		})
	}
}

func TestZeroBatchLength(t *testing.T) {
	trace, err := FromDBModel([]dbmodel.Span{})
	require.NoError(t, err)
	assert.Equal(t, 0, trace.ResourceSpans().Len())
}

func TestEmptySpansAndProcess(t *testing.T) {
	trace, err := FromDBModel([]dbmodel.Span{})
	require.NoError(t, err)
	assert.Equal(t, 0, trace.ResourceSpans().Len())
}

func TestGetStatusCodeFromHTTPStatusAttr(t *testing.T) {
	tests := []struct {
		name string
		attr pcommon.Value
		kind ptrace.SpanKind
		code ptrace.StatusCode
		err  string
	}{
		{
			name: "string-unknown",
			attr: pcommon.NewValueStr("10"),
			kind: ptrace.SpanKindClient,
			code: ptrace.StatusCodeError,
		},

		{
			name: "string-ok",
			attr: pcommon.NewValueStr("101"),
			kind: ptrace.SpanKindClient,
			code: ptrace.StatusCodeUnset,
		},

		{
			name: "int-not-found",
			attr: pcommon.NewValueInt(404),
			kind: ptrace.SpanKindClient,
			code: ptrace.StatusCodeError,
		},
		{
			name: "int-not-found-client-span",
			attr: pcommon.NewValueInt(404),
			kind: ptrace.SpanKindServer,
			code: ptrace.StatusCodeUnset,
		},
		{
			name: "int-invalid-arg",
			attr: pcommon.NewValueInt(408),
			kind: ptrace.SpanKindClient,
			code: ptrace.StatusCodeError,
		},
		{
			name: "int-internal",
			attr: pcommon.NewValueInt(500),
			kind: ptrace.SpanKindClient,
			code: ptrace.StatusCodeError,
		},
		{
			name: "wrong inputValue",
			attr: pcommon.NewValueBool(true),
			kind: ptrace.SpanKindClient,
			code: ptrace.StatusCodeUnset,
			err:  "invalid type: Bool",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, err := getStatusCodeFromHTTPStatusAttr(test.attr, test.kind)
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, test.code, code)
		})
	}
}

func Test_SetSpanEventsFromDbSpanLogs(t *testing.T) {
	traces := ptrace.NewTraces()
	span := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.Events().AppendEmpty().SetName("event1")
	span.Events().AppendEmpty().SetName("event2")
	span.Events().AppendEmpty().Attributes().PutStr(eventNameAttr, "testing")
	logs := []dbmodel.Log{
		{
			Timestamp: model.TimeAsEpochMicroseconds(testSpanEventTime),
		},
		{
			Timestamp: model.TimeAsEpochMicroseconds(testSpanEventTime),
		},
	}
	dbSpanLogsToSpanEvents(logs, span.Events())
	for i := 0; i < len(logs); i++ {
		assert.Equal(t, testSpanEventTime, span.Events().At(i).Timestamp().AsTime())
	}
	assert.Equal(t, 1, span.Events().At(2).Attributes().Len())
	assert.Empty(t, span.Events().At(2).Name())
}

func TestSetAttributesFromDbTags(t *testing.T) {
	wrongValue := "wrong-inputValue"
	tests := []struct {
		name            string
		keyModel        dbmodel.KeyValue
		expectedValueFn func(pcommon.Map)
	}{
		{
			name: "wrong bool input value",
			keyModel: dbmodel.KeyValue{
				Key:   "bool-val",
				Type:  dbmodel.BoolType,
				Value: wrongValue,
			},
			expectedValueFn: func(p pcommon.Map) {
				p.PutStr("bool-val", "Can't convert the type bool for the key bool-val: strconv.ParseBool: parsing \"wrong-inputValue\": invalid syntax")
			},
		},
		{
			name: "right bool input value",
			keyModel: dbmodel.KeyValue{
				Key:   "bool-val",
				Type:  dbmodel.BoolType,
				Value: "true",
			},
			expectedValueFn: func(p pcommon.Map) {
				p.PutBool("bool-val", true)
			},
		},
		{
			name: "non string bool value",
			keyModel: dbmodel.KeyValue{
				Key:   "bool-val",
				Type:  dbmodel.BoolType,
				Value: true,
			},
			expectedValueFn: func(p pcommon.Map) {
				p.PutBool("bool-val", true)
			},
		},
		{
			name: "wrong int input value",
			keyModel: dbmodel.KeyValue{
				Key:   "int-val",
				Type:  dbmodel.Int64Type,
				Value: wrongValue,
			},
			expectedValueFn: func(p pcommon.Map) {
				p.PutStr("int-val", "Can't convert the type int64 for the key int-val: strconv.ParseInt: parsing \"wrong-inputValue\": invalid syntax")
			},
		},
		{
			name: "right int input value",
			keyModel: dbmodel.KeyValue{
				Key:   "int-val",
				Type:  dbmodel.Int64Type,
				Value: "123",
			},
			expectedValueFn: func(p pcommon.Map) {
				p.PutInt("int-val", 123)
			},
		},
		{
			name: "wrong double input value",
			keyModel: dbmodel.KeyValue{
				Key:   "double-val",
				Type:  dbmodel.Float64Type,
				Value: wrongValue,
			},
			expectedValueFn: func(p pcommon.Map) {
				p.PutStr("double-val", "Can't convert the type float64 for the key double-val: strconv.ParseFloat: parsing \"wrong-inputValue\": invalid syntax")
			},
		},
		{
			name: "right double input value",
			keyModel: dbmodel.KeyValue{
				Key:   "double-val",
				Type:  dbmodel.Float64Type,
				Value: "1.23",
			},
			expectedValueFn: func(p pcommon.Map) {
				p.PutDouble("double-val", 1.23)
			},
		},
		{
			name: "wrong binary input value",
			keyModel: dbmodel.KeyValue{
				Key:   "binary-val",
				Type:  dbmodel.BinaryType,
				Value: wrongValue,
			},
			expectedValueFn: func(p pcommon.Map) {
				p.PutStr("binary-val", "Can't convert the type binary for the key binary-val: encoding/hex: invalid byte: U+0077 'w'")
			},
		},
		{
			name: "right binary input value",
			keyModel: dbmodel.KeyValue{
				Key:   "binary-val",
				Type:  dbmodel.BinaryType,
				Value: hex.EncodeToString([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x64, 0x7D, 0x98}),
			},
			expectedValueFn: func(p pcommon.Map) {
				p.PutEmptyBytes("binary-val").FromRaw([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x64, 0x7D, 0x98})
			},
		},
		{
			name: "non-string input value",
			keyModel: dbmodel.KeyValue{
				Key:   "bool-val",
				Type:  dbmodel.Int64Type,
				Value: 123,
			},
			expectedValueFn: func(p pcommon.Map) {
				p.PutStr("bool-val", "Got non string inputValue for the key bool-val")
			},
		},
		{
			name: "right string input value",
			keyModel: dbmodel.KeyValue{
				Key:   "string-val",
				Type:  dbmodel.StringType,
				Value: "right-value",
			},
			expectedValueFn: func(p pcommon.Map) {
				p.PutStr("string-val", "right-value")
			},
		},
		{
			name: "unknown type",
			keyModel: dbmodel.KeyValue{
				Key:   "unknown",
				Type:  dbmodel.ValueType("unknown"),
				Value: "any",
			},
			expectedValueFn: func(p pcommon.Map) {
				p.PutStr("unknown", "<Unknown Jaeger TagType \"unknown\">")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expected := pcommon.NewMap()
			test.expectedValueFn(expected)
			got := pcommon.NewMap()
			dbTagsToAttributes([]dbmodel.KeyValue{test.keyModel}, got)
			assert.Equal(t, expected, got)
		})
	}
}

func TestFromDBModelErrors(t *testing.T) {
	tests := []struct {
		name    string
		err     string
		dbSpans []dbmodel.Span
	}{
		{
			name:    "wrong trace-id",
			dbSpans: []dbmodel.Span{{TraceID: dbmodel.TraceID("trace-id")}},
			err:     "encoding/hex: invalid byte: U+0074 't'",
		},
		{
			name:    "wrong span-id",
			dbSpans: []dbmodel.Span{{SpanID: dbmodel.SpanID("span-id")}},
			err:     "encoding/hex: invalid byte: U+0073 's'",
		},
		{
			name:    "wrong parent span-id",
			dbSpans: []dbmodel.Span{{ParentSpanID: dbmodel.SpanID("parent-span-id")}},
			err:     "encoding/hex: invalid byte: U+0070 'p'",
		},
		{
			name:    "wrong-ref-trace-id",
			dbSpans: []dbmodel.Span{{References: []dbmodel.Reference{{TraceID: dbmodel.TraceID("ref-trace-id")}}}},
			err:     "encoding/hex: invalid byte: U+0072 'r'",
		},
		{
			name:    "wrong-ref-span-id",
			dbSpans: []dbmodel.Span{{References: []dbmodel.Reference{{SpanID: dbmodel.SpanID("ref-span-id")}}}},
			err:     "encoding/hex: invalid byte: U+0072 'r'",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := FromDBModel(test.dbSpans)
			require.ErrorContains(t, err, test.err)
		})
	}
}

func TestSetParentId(t *testing.T) {
	parentSpanId := [8]byte{0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8}
	trace, err := FromDBModel([]dbmodel.Span{{ParentSpanID: getDbSpanIdFromByteArray(parentSpanId)}})
	require.NoError(t, err)
	assert.Equal(t, pcommon.SpanID(parentSpanId), trace.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).ParentSpanID())
}

func TestParentIdWhenRefTraceIdIsDifferent(t *testing.T) {
	traceId := getDbTraceIdFromByteArray([16]byte{0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8, 0xF9, 0xFA, 0xFB, 0xFC, 0xFD, 0xFE, 0xFF, 0x80})
	refTraceId := getDbTraceIdFromByteArray([16]byte{0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8, 0xF9, 0xFA, 0xFB, 0xFC, 0xFD, 0xFE, 0xFF, 0x81})
	trace, err := FromDBModel([]dbmodel.Span{{TraceID: traceId, References: []dbmodel.Reference{{TraceID: refTraceId}}}})
	require.NoError(t, err)
	assert.True(t, trace.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).ParentSpanID().IsEmpty())
}

func TestSetInternalSpanStatus(t *testing.T) {
	okStatus := ptrace.NewStatus()
	okStatus.SetCode(ptrace.StatusCodeOk)

	errorStatus := ptrace.NewStatus()
	errorStatus.SetCode(ptrace.StatusCodeError)

	errorStatusWithMessage := ptrace.NewStatus()
	errorStatusWithMessage.SetCode(ptrace.StatusCodeError)
	errorStatusWithMessage.SetMessage("Error: Invalid argument")

	errorStatusWith404Message := ptrace.NewStatus()
	errorStatusWith404Message.SetCode(ptrace.StatusCodeError)
	errorStatusWith404Message.SetMessage("HTTP 404: Not Found")

	tests := []struct {
		name             string
		attrs            map[string]any
		status           ptrace.Status
		kind             ptrace.SpanKind
		attrsModifiedLen int // Length of attributes map after dropping converted fields
	}{
		{
			name: "status.code is set as string",
			attrs: map[string]any{
				conventions.OtelStatusCode: statusOk,
			},
			status:           okStatus,
			attrsModifiedLen: 0,
		},
		{
			name: "status.code, status.message and error tags are set",
			attrs: map[string]any{
				conventions.OtelStatusCode:        statusError,
				conventions.OtelStatusDescription: "Error: Invalid argument",
			},
			status:           errorStatusWithMessage,
			attrsModifiedLen: 0,
		},
		{
			name: "http.status_code tag is set as string",
			attrs: map[string]any{
				conventions.AttributeHTTPStatusCode: "404",
			},
			status:           errorStatus,
			attrsModifiedLen: 1,
		},
		{
			name: "http.status_code, http.status_message and error tags are set",
			attrs: map[string]any{
				conventions.AttributeHTTPStatusCode: 404,
				tagHTTPStatusMsg:                    "HTTP 404: Not Found",
			},
			status:           errorStatusWith404Message,
			attrsModifiedLen: 2,
		},
		{
			name: "status.code has precedence over http.status_code.",
			attrs: map[string]any{
				conventions.OtelStatusCode:          statusOk,
				conventions.AttributeHTTPStatusCode: 500,
				tagHTTPStatusMsg:                    "Server Error",
			},
			status:           okStatus,
			attrsModifiedLen: 2,
		},
		{
			name: "status.error has precedence over http.status_error.",
			attrs: map[string]any{
				conventions.OtelStatusCode:          statusError,
				conventions.AttributeHTTPStatusCode: 500,
				tagHTTPStatusMsg:                    "Server Error",
			},
			status:           errorStatus,
			attrsModifiedLen: 2,
		},
		{
			name: "whether tagHttpStatusMsg is set as string",
			attrs: map[string]any{
				conventions.AttributeHTTPStatusCode: 404,
				tagHTTPStatusMsg:                    "HTTP 404: Not Found",
			},
			status:           errorStatusWith404Message,
			attrsModifiedLen: 2,
		},
		{
			name: "error tag set and message present",
			attrs: map[string]any{
				tagError:                          true,
				conventions.OtelStatusDescription: "Error: Invalid argument",
			},
			status:           errorStatusWithMessage,
			attrsModifiedLen: 0,
		},
		{
			name: "error tag set and http tag message present",
			attrs: map[string]any{
				tagError:         true,
				tagHTTPStatusMsg: "HTTP 404: Not Found",
			},
			status:           errorStatusWith404Message,
			attrsModifiedLen: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			span.SetKind(test.kind)
			status := span.Status()
			attrs := pcommon.NewMap()
			require.NoError(t, attrs.FromRaw(test.attrs))
			setSpanStatus(attrs, span)
			assert.Equal(t, test.status, status)
			assert.Equal(t, test.attrsModifiedLen, attrs.Len())
		})
	}
}

func TestDBSpanKindToOTELSpanKind(t *testing.T) {
	tests := []struct {
		jSpanKind    string
		otlpSpanKind ptrace.SpanKind
	}{
		{
			jSpanKind:    "client",
			otlpSpanKind: ptrace.SpanKindClient,
		},
		{
			jSpanKind:    "server",
			otlpSpanKind: ptrace.SpanKindServer,
		},
		{
			jSpanKind:    "producer",
			otlpSpanKind: ptrace.SpanKindProducer,
		},
		{
			jSpanKind:    "consumer",
			otlpSpanKind: ptrace.SpanKindConsumer,
		},
		{
			jSpanKind:    "internal",
			otlpSpanKind: ptrace.SpanKindInternal,
		},
		{
			jSpanKind:    "all-others",
			otlpSpanKind: ptrace.SpanKindUnspecified,
		},
	}

	for _, test := range tests {
		t.Run(test.jSpanKind, func(t *testing.T) {
			assert.Equal(t, test.otlpSpanKind, dbSpanKindToOTELSpanKind(test.jSpanKind))
		})
	}
}

func TestFromDbModel_Fixtures(t *testing.T) {
	tracesData, spansData := loadFixtures(t, 1)
	unmarshaller := ptrace.JSONUnmarshaler{}
	expectedTd, err := unmarshaller.UnmarshalTraces(tracesData)
	require.NoError(t, err)
	spans := ToDBModel(expectedTd)
	assert.Len(t, spans, 1)
	testSpans(t, spansData, spans[0])
	actualTd, err := FromDBModel(spans)
	require.NoError(t, err)
	testTraces(t, tracesData, actualTd)
}

func getDbTraceIdFromByteArray(arr [16]byte) dbmodel.TraceID {
	return dbmodel.TraceID(hex.EncodeToString(arr[:]))
}

func getDbSpanIdFromByteArray(arr [8]byte) dbmodel.SpanID {
	return dbmodel.SpanID(hex.EncodeToString(arr[:]))
}

func BenchmarkProtoBatchToInternalTraces(b *testing.B) {
	data, err := os.ReadFile("fixtures.es_01.json")
	require.NoError(b, err)
	var dbSpan dbmodel.Span
	err = json.Unmarshal(data, &dbSpan)
	require.NoError(b, err)
	jb := []dbmodel.Span{dbSpan}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		_, err := FromDBModel(jb)
		assert.NoError(b, err)
	}
}
