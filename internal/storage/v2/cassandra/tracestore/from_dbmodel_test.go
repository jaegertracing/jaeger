// Copyright (c) 2025 The Jaeger Authors.
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Code originally copied from https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/e49500a9b68447cbbe237fa29526ba99e4963f39/pkg/translator/jaeger/jaegerproto_to_traces_test.go

package tracestore

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore/dbmodel"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

// Use timestamp with microsecond granularity to work well with jaeger thrift translation
var testSpanEventTime = time.Date(2020, 2, 11, 20, 26, 13, 123000, time.UTC).UnixMicro()

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
	trace := FromDBModel([]dbmodel.Span{})
	assert.Equal(t, 0, trace.ResourceSpans().Len())
}

func TestEmptyServiceNameAndTags(t *testing.T) {
	tests := []struct {
		name    string
		batches []dbmodel.Span
	}{
		{
			name: "empty service with nil tags",
			batches: []dbmodel.Span{
				{
					Process: dbmodel.Process{
						ServiceName: "",
					},
				},
			},
		},
		{
			name: "empty service with tags",
			batches: []dbmodel.Span{
				{
					Process: dbmodel.Process{
						ServiceName: "",
						Tags:        []dbmodel.KeyValue{},
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			trace := FromDBModel(test.batches)
			assert.Equal(t, 1, trace.ResourceSpans().Len())
			assert.Equal(t, 0, trace.ResourceSpans().At(0).Resource().Attributes().Len())
		})
	}
}

func TestEmptySpansAndProcess(t *testing.T) {
	trace := FromDBModel([]dbmodel.Span{{}})
	assert.Equal(t, 1, trace.ResourceSpans().Len())
}

func Test_dbSpansToSpans_EmptySpans(t *testing.T) {
	spans := []dbmodel.Span{{}}
	traceData := ptrace.NewTraces()
	rss := traceData.ResourceSpans()
	dbSpansToSpans(spans, rss)
	assert.Equal(t, 1, rss.Len())
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
			name: "wrong value",
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

func Test_dbLogsToSpanEvents(t *testing.T) {
	traces := ptrace.NewTraces()
	span := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.Events().AppendEmpty().SetName("event1")
	span.Events().AppendEmpty().SetName("event2")
	span.Events().AppendEmpty().Attributes().PutStr(eventNameAttr, "testing")
	logs := []dbmodel.Log{
		{
			Timestamp: testSpanEventTime,
		},
		{
			Timestamp: testSpanEventTime,
		},
	}
	dbLogsToSpanEvents(logs, span.Events())
	for i := range logs {
		assert.Equal(t, testSpanEventTime, int64(span.Events().At(i).Timestamp()/1000))
	}
	assert.Equal(t, 1, span.Events().At(2).Attributes().Len())
	assert.Empty(t, span.Events().At(2).Name())
}

func Test_dbTagsToAttributes(t *testing.T) {
	tags := []dbmodel.KeyValue{
		{
			Key:       "bool-val",
			ValueType: dbmodel.BoolType,
			ValueBool: true,
		},
		{
			Key:        "int-val",
			ValueType:  dbmodel.Int64Type,
			ValueInt64: 123,
		},
		{
			Key:         "string-val",
			ValueType:   dbmodel.StringType,
			ValueString: "abc",
		},
		{
			Key:          "double-val",
			ValueType:    dbmodel.Float64Type,
			ValueFloat64: 1.23,
		},
		{
			Key:         "binary-val",
			ValueType:   dbmodel.BinaryType,
			ValueBinary: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x64, 0x7D, 0x98},
		},
		{
			Key:       "testing-key",
			ValueType: "some random value",
		},
	}

	expected := pcommon.NewMap()
	expected.PutBool("bool-val", true)
	expected.PutInt("int-val", 123)
	expected.PutStr("string-val", "abc")
	expected.PutDouble("double-val", 1.23)
	expected.PutEmptyBytes("binary-val").FromRaw([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x64, 0x7D, 0x98})
	expected.PutStr("testing-key", "<Unknown Jaeger TagType \"some random value\">")

	got := pcommon.NewMap()
	dbTagsToAttributes(tags, got)

	require.Equal(t, expected, got)
}

func TestSetInternalSpanStatus(t *testing.T) {
	emptyStatus := ptrace.NewStatus()

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
			name:             "No tags set -> OK status",
			status:           emptyStatus,
			attrsModifiedLen: 0,
		},
		{
			name: "error tag set -> Error status",
			attrs: map[string]any{
				tagError: true,
			},
			status:           errorStatus,
			attrsModifiedLen: 0,
		},
		{
			name: "status.code is set as string",
			attrs: map[string]any{
				otelsemconv.OtelStatusCode: statusOk,
			},
			status:           okStatus,
			attrsModifiedLen: 0,
		},
		{
			name: "status.code, status.message and error tags are set",
			attrs: map[string]any{
				tagError:                          true,
				otelsemconv.OtelStatusCode:        statusError,
				otelsemconv.OtelStatusDescription: "Error: Invalid argument",
			},
			status:           errorStatusWithMessage,
			attrsModifiedLen: 0,
		},
		{
			name: "http.status_code tag is set as string",
			attrs: map[string]any{
				otelsemconv.HTTPResponseStatusCodeKey: "404",
			},
			status:           errorStatus,
			attrsModifiedLen: 1,
		},
		{
			name: "http.status_code, http.status_message and error tags are set",
			attrs: map[string]any{
				tagError:                              true,
				otelsemconv.HTTPResponseStatusCodeKey: 404,
				tagHTTPStatusMsg:                      "HTTP 404: Not Found",
			},
			status:           errorStatusWith404Message,
			attrsModifiedLen: 2,
		},
		{
			name: "status.code has precedence over http.status_code.",
			attrs: map[string]any{
				otelsemconv.OtelStatusCode:            statusOk,
				otelsemconv.HTTPResponseStatusCodeKey: 500,
				tagHTTPStatusMsg:                      "Server Error",
			},
			status:           okStatus,
			attrsModifiedLen: 2,
		},
		{
			name: "Ignore http.status_code == 200 if error set to true.",
			attrs: map[string]any{
				tagError:                              true,
				otelsemconv.HTTPResponseStatusCodeKey: http.StatusOK,
			},
			status:           errorStatus,
			attrsModifiedLen: 1,
		},
		{
			name: "status.error has precedence over http.status_error.",
			attrs: map[string]any{
				otelsemconv.OtelStatusCode:            statusError,
				otelsemconv.HTTPResponseStatusCodeKey: 500,
				tagHTTPStatusMsg:                      "Server Error",
			},
			status:           errorStatus,
			attrsModifiedLen: 2,
		},
		{
			name: "the 4xx range span status MUST be left unset in case of SpanKind.SERVER",
			kind: ptrace.SpanKindServer,
			attrs: map[string]any{
				tagError:                              false,
				otelsemconv.HTTPResponseStatusCodeKey: 404,
			},
			status:           emptyStatus,
			attrsModifiedLen: 2,
		},
		{
			name: "whether tagHttpStatusMsg is set as string",
			attrs: map[string]any{
				otelsemconv.HTTPResponseStatusCodeKey: 404,
				tagHTTPStatusMsg:                      "HTTP 404: Not Found",
			},
			status:           errorStatusWith404Message,
			attrsModifiedLen: 2,
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

func TestJSpanKindToInternal(t *testing.T) {
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
			assert.Equal(t, test.otlpSpanKind, jSpanKindToInternal(test.jSpanKind))
		})
	}
}

func BenchmarkProtoBatchToInternalTraces(b *testing.B) {
	data, err := os.ReadFile("fixtures/cas_01.json")
	require.NoError(b, err)
	var batch dbmodel.Span
	err = json.Unmarshal(data, &batch)
	require.NoError(b, err)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		FromDBModel([]dbmodel.Span{batch})
	}
}

func TestFromDbModel_Fixtures(t *testing.T) {
	tracesStr, batchStr := loadFixtures(t, 1)
	var batch dbmodel.Span
	err := json.Unmarshal(batchStr, &batch)
	require.NoError(t, err)
	td := FromDBModel([]dbmodel.Span{batch})
	testTraces(t, tracesStr, td)
	batches := ToDBModel(td)
	assert.Len(t, batches, 1)
	testSpans(t, batchStr, batches[0])
}
