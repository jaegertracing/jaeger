// Copyright (c) 2025 The Jaeger Authors.
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Code originally copied from https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/e49500a9b68447cbbe237fa29526ba99e4963f39/pkg/translator/jaeger/jaegerproto_to_traces_test.go

package tracestore

import (
	"encoding/binary"
	"net/http"
	"strconv"
	"testing"
	"time"

	idutils "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/core/xidutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	conventions "go.opentelemetry.io/collector/semconv/v1.16.0"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

// Use timespamp with microsecond granularity to work well with jaeger thrift translation
var (
	testSpanStartTime      = time.Date(2020, 2, 11, 20, 26, 12, 321000, time.UTC)
	testSpanStartTimestamp = pcommon.NewTimestampFromTime(testSpanStartTime)
	testSpanEventTime      = time.Date(2020, 2, 11, 20, 26, 13, 123000, time.UTC)
	testSpanEventTimestamp = pcommon.NewTimestampFromTime(testSpanEventTime)
	testSpanEndTime        = time.Date(2020, 2, 11, 20, 26, 13, 789000, time.UTC)
	testSpanEndTimestamp   = pcommon.NewTimestampFromTime(testSpanEndTime)
)

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
	trace, err := ProtoToTraces([]*model.Batch{})
	require.NoError(t, err)
	assert.Equal(t, 0, trace.ResourceSpans().Len())
}

func TestEmptyServiceNameAndTags(t *testing.T) {
	tests := []struct {
		name    string
		batches []*model.Batch
	}{
		{
			name: "empty service with nil tags",
			batches: []*model.Batch{
				{
					Process: &model.Process{
						ServiceName: "",
					},
				},
			},
		},
		{
			name: "empty service with tags",
			batches: []*model.Batch{
				{
					Process: &model.Process{
						ServiceName: "",
						Tags:        []model.KeyValue{},
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			trace, err := ProtoToTraces(test.batches)
			require.NoError(t, err)
			assert.Equal(t, 1, trace.ResourceSpans().Len())
			assert.Equal(t, 0, trace.ResourceSpans().At(0).Resource().Attributes().Len())
		})
	}
}

func TestEmptySpansAndProcess(t *testing.T) {
	trace, err := ProtoToTraces([]*model.Batch{{Spans: []*model.Span{}}})
	require.NoError(t, err)
	assert.Equal(t, 0, trace.ResourceSpans().Len())
}

func Test_translateHostnameAttr(t *testing.T) {
	traceData := ptrace.NewTraces()
	rss := traceData.ResourceSpans().AppendEmpty().Resource().Attributes()
	rss.PutStr("hostname", "testing")
	translateHostnameAttr(rss)
	_, hostNameFound := rss.Get("hostname")
	assert.False(t, hostNameFound)
	convHostName, convHostNameFound := rss.Get(conventions.AttributeHostName)
	assert.True(t, convHostNameFound)
	assert.Equal(t, "testing", convHostName.AsString())
}

func Test_translateJaegerVersionAttr(t *testing.T) {
	traceData := ptrace.NewTraces()
	rss := traceData.ResourceSpans().AppendEmpty().Resource().Attributes()
	rss.PutStr("jaeger.version", "1.0.0")
	translateJaegerVersionAttr(rss)
	_, jaegerVersionFound := rss.Get("jaeger.version")
	assert.False(t, jaegerVersionFound)
	exportVersion, exportVersionFound := rss.Get(attributeExporterVersion)
	assert.True(t, exportVersionFound)
	assert.Equal(t, "Jaeger-1.0.0", exportVersion.AsString())
}

func Test_jSpansToInternal_EmptyOrNilSpans(t *testing.T) {
	tests := []struct {
		name  string
		spans []*model.Span
	}{
		{
			name:  "nil spans",
			spans: []*model.Span{nil},
		},
		{
			name:  "empty spans",
			spans: []*model.Span{new(model.Span)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			traceData := ptrace.NewTraces()
			rss := traceData.ResourceSpans().AppendEmpty().ScopeSpans()
			jSpansToInternal(tt.spans, rss)
			assert.Equal(t, 0, rss.Len())
		})
	}
}

func Test_jTagsToInternalAttributes(t *testing.T) {
	traceData := ptrace.NewTraces()
	rss := traceData.ResourceSpans().AppendEmpty().Resource().Attributes()
	kv := []model.KeyValue{{
		Key:   "testing-key",
		VType: model.ValueType(12),
	}}
	jTagsToInternalAttributes(kv, rss)
	testingKey, testingKeyFound := rss.Get("testing-key")
	assert.True(t, testingKeyFound)
	assert.Equal(t, "<Unknown Jaeger TagType \"12\">", testingKey.AsString())
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

func Test_jLogsToSpanEvents(t *testing.T) {
	traces := ptrace.NewTraces()
	span := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.Events().AppendEmpty().SetName("event1")
	span.Events().AppendEmpty().SetName("event2")
	span.Events().AppendEmpty().Attributes().PutStr(eventNameAttr, "testing")
	logs := []model.Log{
		{
			Timestamp: testSpanEventTime,
		},
		{
			Timestamp: testSpanEventTime,
		},
	}
	jLogsToSpanEvents(logs, span.Events())
	for i := 0; i < len(logs); i++ {
		assert.Equal(t, testSpanEventTime, span.Events().At(i).Timestamp().AsTime())
	}
	assert.Equal(t, 1, span.Events().At(2).Attributes().Len())
	assert.Empty(t, span.Events().At(2).Name())
}

func TestJTagsToInternalAttributes(t *testing.T) {
	tags := []model.KeyValue{
		{
			Key:   "bool-val",
			VType: model.ValueType_BOOL,
			VBool: true,
		},
		{
			Key:    "int-val",
			VType:  model.ValueType_INT64,
			VInt64: 123,
		},
		{
			Key:   "string-val",
			VType: model.ValueType_STRING,
			VStr:  "abc",
		},
		{
			Key:      "double-val",
			VType:    model.ValueType_FLOAT64,
			VFloat64: 1.23,
		},
		{
			Key:     "binary-val",
			VType:   model.ValueType_BINARY,
			VBinary: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x64, 0x7D, 0x98},
		},
	}

	expected := pcommon.NewMap()
	expected.PutBool("bool-val", true)
	expected.PutInt("int-val", 123)
	expected.PutStr("string-val", "abc")
	expected.PutDouble("double-val", 1.23)
	expected.PutEmptyBytes("binary-val").FromRaw([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x64, 0x7D, 0x98})

	got := pcommon.NewMap()
	jTagsToInternalAttributes(tags, got)

	require.Equal(t, expected, got)
}

func TestProtoToTraces(t *testing.T) {
	tests := []struct {
		name string
		jb   []*model.Batch
		td   ptrace.Traces
	}{
		{
			name: "empty",
			jb:   []*model.Batch{},
			td:   ptrace.NewTraces(),
		},

		{
			name: "no-spans",
			jb: []*model.Batch{
				{
					Process: generateProtoProcess(),
				},
			},
			td: generateTracesResourceOnly(),
		},

		{
			name: "no-resource-attrs",
			jb: []*model.Batch{
				{
					Process: &model.Process{
						ServiceName: noServiceName,
					},
				},
			},
			td: generateTracesResourceOnlyWithNoAttrs(),
		},

		{
			name: "one-span-no-resources",
			jb: []*model.Batch{
				{
					Process: &model.Process{
						ServiceName: noServiceName,
					},
					Spans: []*model.Span{
						generateProtoSpanWithTraceState(),
					},
				},
			},
			td: generateTracesOneSpanNoResourceWithTraceState(),
		},
		{
			name: "two-spans-child-parent",
			jb: []*model.Batch{
				{
					Process: &model.Process{
						ServiceName: noServiceName,
					},
					Spans: []*model.Span{
						generateProtoSpan(),
						generateProtoChildSpan(),
					},
				},
			},
			td: generateTracesTwoSpansChildParent(),
		},

		{
			name: "two-spans-with-follower",
			jb: []*model.Batch{
				{
					Process: &model.Process{
						ServiceName: noServiceName,
					},
					Spans: []*model.Span{
						generateProtoSpan(),
						generateProtoFollowerSpan(),
					},
				},
			},
			td: generateTracesTwoSpansWithFollower(),
		},
		{
			name: "a-spans-with-two-parent",
			jb: []*model.Batch{
				{
					Process: &model.Process{
						ServiceName: noServiceName,
					},
					Spans: []*model.Span{
						generateProtoSpan(),
						generateProtoFollowerSpan(),
						generateProtoTwoParentsSpan(),
					},
				},
			},
			td: generateTracesSpanWithTwoParents(),
		},
		{
			name: "no-error-from-server-span-with-4xx-http-code",
			jb: []*model.Batch{
				{
					Process: &model.Process{
						ServiceName: noServiceName,
					},
					Spans: []*model.Span{
						{
							StartTime: testSpanStartTime,
							Duration:  testSpanEndTime.Sub(testSpanStartTime),
							Tags: []model.KeyValue{
								{
									Key:   model.SpanKindKey,
									VType: model.ValueType_STRING,
									VStr:  string(model.SpanKindServer),
								},
								{
									Key:   conventions.AttributeHTTPStatusCode,
									VType: model.ValueType_STRING,
									VStr:  "404",
								},
							},
						},
					},
				},
			},
			td: func() ptrace.Traces {
				traces := ptrace.NewTraces()
				span := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
				span.SetStartTimestamp(testSpanStartTimestamp)
				span.SetEndTimestamp(testSpanEndTimestamp)
				span.SetKind(ptrace.SpanKindClient)
				span.SetKind(ptrace.SpanKindServer)
				span.Status().SetCode(ptrace.StatusCodeUnset)
				span.Attributes().PutStr(conventions.AttributeHTTPStatusCode, "404")
				return traces
			}(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			td, err := ProtoToTraces(test.jb)
			require.NoError(t, err)
			assert.Equal(t, test.td, td)
		})
	}
}

func TestProtoBatchToInternalTracesWithTwoLibraries(t *testing.T) {
	jb := &model.Batch{
		Process: &model.Process{
			ServiceName: noServiceName,
		},
		Spans: []*model.Span{
			{
				StartTime:     testSpanStartTime,
				Duration:      testSpanEndTime.Sub(testSpanStartTime),
				OperationName: "operation2",
				Tags: []model.KeyValue{
					{
						Key:   conventions.AttributeOtelScopeName,
						VType: model.ValueType_STRING,
						VStr:  "library2",
					}, {
						Key:   conventions.AttributeOtelScopeVersion,
						VType: model.ValueType_STRING,
						VStr:  "0.42.0",
					},
				},
			},
			{
				TraceID:       model.NewTraceID(0, 0),
				StartTime:     testSpanStartTime,
				Duration:      testSpanEndTime.Sub(testSpanStartTime),
				OperationName: "operation1",
				Tags: []model.KeyValue{
					{
						Key:   conventions.AttributeOtelScopeName,
						VType: model.ValueType_STRING,
						VStr:  "library1",
					}, {
						Key:   conventions.AttributeOtelScopeVersion,
						VType: model.ValueType_STRING,
						VStr:  "0.42.0",
					},
				},
			},
		},
	}
	expected := generateTracesTwoSpansFromTwoLibraries()
	library1Span := expected.ResourceSpans().At(0).ScopeSpans().At(0)
	library2Span := expected.ResourceSpans().At(0).ScopeSpans().At(1)

	actual, err := ProtoToTraces([]*model.Batch{jb})
	require.NoError(t, err)

	assert.Equal(t, 1, actual.ResourceSpans().Len())
	assert.Equal(t, 2, actual.ResourceSpans().At(0).ScopeSpans().Len())

	ils0 := actual.ResourceSpans().At(0).ScopeSpans().At(0)
	ils1 := actual.ResourceSpans().At(0).ScopeSpans().At(1)
	if ils0.Scope().Name() == "library1" {
		assert.Equal(t, library1Span, ils0)
		assert.Equal(t, library2Span, ils1)
	} else {
		assert.Equal(t, library1Span, ils1)
		assert.Equal(t, library2Span, ils0)
	}
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
				conventions.OtelStatusCode: statusOk,
			},
			status:           okStatus,
			attrsModifiedLen: 0,
		},
		{
			name: "status.code, status.message and error tags are set",
			attrs: map[string]any{
				tagError:                          true,
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
				tagError:                            true,
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
			name: "Ignore http.status_code == 200 if error set to true.",
			attrs: map[string]any{
				tagError:                            true,
				conventions.AttributeHTTPStatusCode: http.StatusOK,
			},
			status:           errorStatus,
			attrsModifiedLen: 1,
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
			name: "the 4xx range span status MUST be left unset in case of SpanKind.SERVER",
			kind: ptrace.SpanKindServer,
			attrs: map[string]any{
				tagError:                            false,
				conventions.AttributeHTTPStatusCode: 404,
			},
			status:           emptyStatus,
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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			span.SetKind(test.kind)
			status := span.Status()
			attrs := pcommon.NewMap()
			require.NoError(t, attrs.FromRaw(test.attrs))
			setInternalSpanStatus(attrs, span)
			assert.Equal(t, test.status, status)
			assert.Equal(t, test.attrsModifiedLen, attrs.Len())
		})
	}
}

func TestProtoBatchesToInternalTraces(t *testing.T) {
	batches := []*model.Batch{
		{
			Process: generateProtoProcess(),
			Spans: []*model.Span{
				generateProtoSpan(),
			},
		},
		{
			Spans: []*model.Span{
				generateProtoSpan(),
				generateProtoChildSpan(),
			},
		},
		{
			// should be skipped
			Spans: []*model.Span{},
		},
	}

	expected := generateTracesOneSpanNoResource()
	resource := generateTracesResourceOnly().ResourceSpans().At(0).Resource()
	resource.CopyTo(expected.ResourceSpans().At(0).Resource())
	tgt := expected.ResourceSpans().AppendEmpty()
	twoSpans := generateTracesTwoSpansChildParent().ResourceSpans().At(0)
	twoSpans.CopyTo(tgt)

	got, err := ProtoToTraces(batches)

	require.NoError(t, err)

	assert.Equal(t, expected.ResourceSpans().Len(), got.ResourceSpans().Len())
	assert.Equal(t, expected.SpanCount(), got.SpanCount())

	lenbatches := expected.ResourceSpans().Len()
	found := 0

	for i := 0; i < lenbatches; i++ {
		rsExpected := expected.ResourceSpans().At(i)
		for j := 0; j < lenbatches; j++ {
			got.ResourceSpans().RemoveIf(func(_ ptrace.ResourceSpans) bool {
				nameExpected := rsExpected.ScopeSpans().At(0).Spans().At(0).Name()
				nameGot := got.ResourceSpans().At(j).ScopeSpans().At(0).Scope().Name()
				if nameExpected == nameGot {
					assert.Equal(t, nameGot, found)
					assert.Equal(t, got.SpanCount(), found)
				}
				return nameExpected == nameGot
			})
		}
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

func TestRegroup(t *testing.T) {
	// prepare
	process := &model.Process{
		ServiceName: "batch-process",
	}
	spanWithoutProcess := &model.Span{
		OperationName: "span-without-process",
	}
	spanWithProcess := &model.Span{
		Process: &model.Process{
			ServiceName: "custom-service-name",
		},
	}

	originalBatches := []*model.Batch{
		{
			Process: process,
			Spans:   []*model.Span{spanWithProcess, spanWithoutProcess},
		},
	}

	expected := []*model.Batch{
		{
			Process: process,
			Spans:   []*model.Span{spanWithoutProcess},
		},
		{
			Process: spanWithProcess.Process,
			Spans:   []*model.Span{spanWithProcess},
		},
	}

	// test
	result := regroup(originalBatches)

	// verify
	assert.ElementsMatch(t, expected, result)
}

func TestChecksum(t *testing.T) {
	testCases := []struct {
		desc     string
		input    *model.Process
		expected uint64
	}{
		{
			desc: "valid process",
			input: &model.Process{
				ServiceName: "some-service-name",
			},
			expected: 0x974574e8529af5dd, // acquired by running it once
		},
		{
			desc:     "nil process",
			input:    nil,
			expected: 0xcbf29ce484222325, // acquired by running it once
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			out := checksum(tC.input)
			assert.Equal(t, tC.expected, out)
		})
	}
}

func generateTracesResourceOnly() ptrace.Traces {
	td := generateTracesOneEmptyResourceSpans()
	rs := td.ResourceSpans().At(0).Resource()
	rs.Attributes().PutStr(conventions.AttributeServiceName, "service-1")
	rs.Attributes().PutInt("int-attr-1", 123)
	return td
}

func generateTracesOneEmptyResourceSpans() ptrace.Traces {
	td := ptrace.NewTraces()
	td.ResourceSpans().AppendEmpty()
	return td
}

func generateTracesResourceOnlyWithNoAttrs() ptrace.Traces {
	return generateTracesOneEmptyResourceSpans()
}

func generateProtoProcess() *model.Process {
	return &model.Process{
		ServiceName: "service-1",
		Tags: []model.KeyValue{
			{
				Key:    "int-attr-1",
				VType:  model.ValueType_INT64,
				VInt64: 123,
			},
		},
	}
}

func GenerateTracesOneSpanNoResource() ptrace.Traces {
	td := generateTracesOneEmptyResourceSpans()
	rs0 := td.ResourceSpans().At(0)
	fillSpanOne(rs0.ScopeSpans().AppendEmpty().Spans().AppendEmpty())
	return td
}

func fillSpanOne(span ptrace.Span) {
	span.SetName("operationA")
	span.SetStartTimestamp(testSpanStartTimestamp)
	span.SetEndTimestamp(testSpanEndTimestamp)
	span.SetDroppedAttributesCount(1)
	evs := span.Events()
	ev0 := evs.AppendEmpty()
	ev0.SetTimestamp(testSpanEventTimestamp)
	ev0.SetName("event-with-attr")
	ev0.Attributes().PutStr("span-event-attr", "span-event-attr-val")
	ev0.SetDroppedAttributesCount(2)
	ev1 := evs.AppendEmpty()
	ev1.SetTimestamp(testSpanEventTimestamp)
	ev1.SetName("event")
	ev1.SetDroppedAttributesCount(2)
	span.SetDroppedEventsCount(1)
	status := span.Status()
	status.SetCode(ptrace.StatusCodeError)
	status.SetMessage("status-cancelled")
}

func generateTracesOneSpanNoResource() ptrace.Traces {
	td := GenerateTracesOneSpanNoResource()
	span := td.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	span.SetSpanID([8]byte{0xAF, 0xAE, 0xAD, 0xAC, 0xAB, 0xAA, 0xA9, 0xA8})
	span.SetTraceID(
		[16]byte{0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8, 0xF9, 0xFA, 0xFB, 0xFC, 0xFD, 0xFE, 0xFF, 0x80})
	span.SetDroppedAttributesCount(0)
	span.SetDroppedEventsCount(0)
	span.SetStartTimestamp(testSpanStartTimestamp)
	span.SetEndTimestamp(testSpanEndTimestamp)
	span.SetKind(ptrace.SpanKindClient)
	span.Status().SetCode(ptrace.StatusCodeError)
	span.Events().At(0).SetTimestamp(testSpanEventTimestamp)
	span.Events().At(0).SetDroppedAttributesCount(0)
	span.Events().At(0).SetName("event-with-attr")
	span.Events().At(1).SetTimestamp(testSpanEventTimestamp)
	span.Events().At(1).SetDroppedAttributesCount(0)
	span.Events().At(1).SetName("")
	span.Events().At(1).Attributes().PutInt("attr-int", 123)
	return td
}

func generateTracesWithLibraryInfo() ptrace.Traces {
	td := generateTracesOneSpanNoResource()
	rs0 := td.ResourceSpans().At(0)
	rs0ils0 := rs0.ScopeSpans().At(0)
	rs0ils0.Scope().SetName("io.opentelemetry.test")
	rs0ils0.Scope().SetVersion("0.42.0")
	return td
}

func generateTracesOneSpanNoResourceWithTraceState() ptrace.Traces {
	td := generateTracesOneSpanNoResource()
	span := td.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	span.TraceState().FromRaw("lasterror=f39cd56cc44274fd5abd07ef1164246d10ce2955")
	return td
}

func generateProtoSpan() *model.Span {
	return &model.Span{
		TraceID: model.NewTraceID(
			binary.BigEndian.Uint64([]byte{0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8}),
			binary.BigEndian.Uint64([]byte{0xF9, 0xFA, 0xFB, 0xFC, 0xFD, 0xFE, 0xFF, 0x80}),
		),
		SpanID:        model.NewSpanID(binary.BigEndian.Uint64([]byte{0xAF, 0xAE, 0xAD, 0xAC, 0xAB, 0xAA, 0xA9, 0xA8})),
		OperationName: "operationA",
		StartTime:     testSpanStartTime,
		Duration:      testSpanEndTime.Sub(testSpanStartTime),
		Logs: []model.Log{
			{
				Timestamp: testSpanEventTime,
				Fields: []model.KeyValue{
					{
						Key:   eventNameAttr,
						VType: model.ValueType_STRING,
						VStr:  "event-with-attr",
					},
					{
						Key:   "span-event-attr",
						VType: model.ValueType_STRING,
						VStr:  "span-event-attr-val",
					},
				},
			},
			{
				Timestamp: testSpanEventTime,
				Fields: []model.KeyValue{
					{
						Key:    "attr-int",
						VType:  model.ValueType_INT64,
						VInt64: 123,
					},
				},
			},
		},
		Tags: []model.KeyValue{
			{
				Key:   model.SpanKindKey,
				VType: model.ValueType_STRING,
				VStr:  string(model.SpanKindClient),
			},
			{
				Key:   conventions.OtelStatusCode,
				VType: model.ValueType_STRING,
				VStr:  statusError,
			},
			{
				Key:   tagError,
				VBool: true,
				VType: model.ValueType_BOOL,
			},
			{
				Key:   conventions.OtelStatusDescription,
				VType: model.ValueType_STRING,
				VStr:  "status-cancelled",
			},
		},
	}
}

func generateProtoSpanWithLibraryInfo(libraryName string) *model.Span {
	span := generateProtoSpan()
	span.Tags = append([]model.KeyValue{
		{
			Key:   conventions.AttributeOtelScopeName,
			VType: model.ValueType_STRING,
			VStr:  libraryName,
		}, {
			Key:   conventions.AttributeOtelScopeVersion,
			VType: model.ValueType_STRING,
			VStr:  "0.42.0",
		},
	}, span.Tags...)

	return span
}

func generateProtoSpanWithTraceState() *model.Span {
	return &model.Span{
		TraceID: model.NewTraceID(
			binary.BigEndian.Uint64([]byte{0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8}),
			binary.BigEndian.Uint64([]byte{0xF9, 0xFA, 0xFB, 0xFC, 0xFD, 0xFE, 0xFF, 0x80}),
		),
		SpanID:        model.NewSpanID(binary.BigEndian.Uint64([]byte{0xAF, 0xAE, 0xAD, 0xAC, 0xAB, 0xAA, 0xA9, 0xA8})),
		OperationName: "operationA",
		StartTime:     testSpanStartTime,
		Duration:      testSpanEndTime.Sub(testSpanStartTime),
		Logs: []model.Log{
			{
				Timestamp: testSpanEventTime,
				Fields: []model.KeyValue{
					{
						Key:   eventNameAttr,
						VType: model.ValueType_STRING,
						VStr:  "event-with-attr",
					},
					{
						Key:   "span-event-attr",
						VType: model.ValueType_STRING,
						VStr:  "span-event-attr-val",
					},
				},
			},
			{
				Timestamp: testSpanEventTime,
				Fields: []model.KeyValue{
					{
						Key:    "attr-int",
						VType:  model.ValueType_INT64,
						VInt64: 123,
					},
				},
			},
		},
		Tags: []model.KeyValue{
			{
				Key:   model.SpanKindKey,
				VType: model.ValueType_STRING,
				VStr:  string(model.SpanKindClient),
			},
			{
				Key:   conventions.OtelStatusCode,
				VType: model.ValueType_STRING,
				VStr:  statusError,
			},
			{
				Key:   tagError,
				VBool: true,
				VType: model.ValueType_BOOL,
			},
			{
				Key:   conventions.OtelStatusDescription,
				VType: model.ValueType_STRING,
				VStr:  "status-cancelled",
			},
			{
				Key:   tagW3CTraceState,
				VType: model.ValueType_STRING,
				VStr:  "lasterror=f39cd56cc44274fd5abd07ef1164246d10ce2955",
			},
		},
	}
}

func generateTracesTwoSpansChildParent() ptrace.Traces {
	td := generateTracesOneSpanNoResource()
	spans := td.ResourceSpans().At(0).ScopeSpans().At(0).Spans()

	span := spans.AppendEmpty()
	span.SetName("operationB")
	span.SetSpanID([8]byte{0x1F, 0x1E, 0x1D, 0x1C, 0x1B, 0x1A, 0x19, 0x18})
	span.SetParentSpanID(spans.At(0).SpanID())
	span.SetKind(ptrace.SpanKindServer)
	span.SetTraceID(spans.At(0).TraceID())
	span.SetStartTimestamp(spans.At(0).StartTimestamp())
	span.SetEndTimestamp(spans.At(0).EndTimestamp())
	span.Status().SetCode(ptrace.StatusCodeUnset)
	span.Attributes().PutInt(conventions.AttributeHTTPStatusCode, 404)
	return td
}

func generateProtoChildSpan() *model.Span {
	traceID := model.NewTraceID(
		binary.BigEndian.Uint64([]byte{0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8}),
		binary.BigEndian.Uint64([]byte{0xF9, 0xFA, 0xFB, 0xFC, 0xFD, 0xFE, 0xFF, 0x80}),
	)
	return &model.Span{
		TraceID:       traceID,
		SpanID:        model.NewSpanID(binary.BigEndian.Uint64([]byte{0x1F, 0x1E, 0x1D, 0x1C, 0x1B, 0x1A, 0x19, 0x18})),
		OperationName: "operationB",
		StartTime:     testSpanStartTime,
		Duration:      testSpanEndTime.Sub(testSpanStartTime),
		Tags: []model.KeyValue{
			{
				Key:    conventions.AttributeHTTPStatusCode,
				VType:  model.ValueType_INT64,
				VInt64: 404,
			},
			{
				Key:   model.SpanKindKey,
				VType: model.ValueType_STRING,
				VStr:  string(model.SpanKindServer),
			},
		},
		References: []model.SpanRef{
			{
				TraceID: traceID,
				SpanID:  model.NewSpanID(binary.BigEndian.Uint64([]byte{0xAF, 0xAE, 0xAD, 0xAC, 0xAB, 0xAA, 0xA9, 0xA8})),
				RefType: model.SpanRefType_CHILD_OF,
			},
		},
	}
}

func generateTracesTwoSpansWithFollower() ptrace.Traces {
	td := generateTracesOneSpanNoResource()
	spans := td.ResourceSpans().At(0).ScopeSpans().At(0).Spans()

	span := spans.AppendEmpty()
	span.SetName("operationC")
	span.SetSpanID([8]byte{0x1F, 0x1E, 0x1D, 0x1C, 0x1B, 0x1A, 0x19, 0x18})
	span.SetTraceID(spans.At(0).TraceID())
	span.SetParentSpanID(spans.At(0).SpanID())
	span.SetStartTimestamp(spans.At(0).EndTimestamp())
	span.SetEndTimestamp(spans.At(0).EndTimestamp() + 1000000)
	span.SetKind(ptrace.SpanKindConsumer)
	span.Status().SetCode(ptrace.StatusCodeOk)
	span.Status().SetMessage("status-ok")
	link := span.Links().AppendEmpty()
	link.SetTraceID(span.TraceID())
	link.SetSpanID(spans.At(0).SpanID())
	link.Attributes().PutStr(
		conventions.AttributeOpentracingRefType,
		conventions.AttributeOpentracingRefTypeFollowsFrom,
	)
	return td
}

func generateProtoFollowerSpan() *model.Span {
	traceID := model.NewTraceID(
		binary.BigEndian.Uint64([]byte{0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8}),
		binary.BigEndian.Uint64([]byte{0xF9, 0xFA, 0xFB, 0xFC, 0xFD, 0xFE, 0xFF, 0x80}),
	)
	return &model.Span{
		TraceID:       traceID,
		SpanID:        model.NewSpanID(binary.BigEndian.Uint64([]byte{0x1F, 0x1E, 0x1D, 0x1C, 0x1B, 0x1A, 0x19, 0x18})),
		OperationName: "operationC",
		StartTime:     testSpanEndTime,
		Duration:      time.Millisecond,
		Tags: []model.KeyValue{
			{
				Key:   model.SpanKindKey,
				VType: model.ValueType_STRING,
				VStr:  string(model.SpanKindConsumer),
			},
			{
				Key:   conventions.OtelStatusCode,
				VType: model.ValueType_STRING,
				VStr:  statusOk,
			},
			{
				Key:   conventions.OtelStatusDescription,
				VType: model.ValueType_STRING,
				VStr:  "status-ok",
			},
		},
		References: []model.SpanRef{
			{
				TraceID: traceID,
				SpanID:  model.NewSpanID(binary.BigEndian.Uint64([]byte{0xAF, 0xAE, 0xAD, 0xAC, 0xAB, 0xAA, 0xA9, 0xA8})),
				RefType: model.SpanRefType_FOLLOWS_FROM,
			},
		},
	}
}

func generateTracesSpanWithTwoParents() ptrace.Traces {
	td := generateTracesTwoSpansWithFollower()
	spans := td.ResourceSpans().At(0).ScopeSpans().At(0).Spans()
	parent := spans.At(0)
	parent2 := spans.At(1)
	span := spans.AppendEmpty()
	span.SetName("operationD")
	span.SetSpanID([8]byte{0x1F, 0x1E, 0x1D, 0x1C, 0x1B, 0x1A, 0x19, 0x20})
	span.SetTraceID(parent.TraceID())
	span.SetStartTimestamp(parent.StartTimestamp())
	span.SetEndTimestamp(parent.EndTimestamp())
	span.SetParentSpanID(parent.SpanID())
	span.SetKind(ptrace.SpanKindConsumer)
	span.Status().SetCode(ptrace.StatusCodeOk)
	span.Status().SetMessage("status-ok")

	link := span.Links().AppendEmpty()
	link.SetTraceID(parent2.TraceID())
	link.SetSpanID(parent2.SpanID())
	link.Attributes().PutStr(
		conventions.AttributeOpentracingRefType,
		conventions.AttributeOpentracingRefTypeChildOf,
	)
	return td
}

func generateProtoTwoParentsSpan() *model.Span {
	traceID := model.NewTraceID(
		binary.BigEndian.Uint64([]byte{0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8}),
		binary.BigEndian.Uint64([]byte{0xF9, 0xFA, 0xFB, 0xFC, 0xFD, 0xFE, 0xFF, 0x80}),
	)
	return &model.Span{
		TraceID:       traceID,
		SpanID:        model.NewSpanID(binary.BigEndian.Uint64([]byte{0x1F, 0x1E, 0x1D, 0x1C, 0x1B, 0x1A, 0x19, 0x20})),
		OperationName: "operationD",
		StartTime:     testSpanStartTime,
		Duration:      testSpanEndTime.Sub(testSpanStartTime),
		Tags: []model.KeyValue{
			{
				Key:   model.SpanKindKey,
				VType: model.ValueType_STRING,
				VStr:  string(model.SpanKindConsumer),
			},
			{
				Key:   conventions.OtelStatusCode,
				VType: model.ValueType_STRING,
				VStr:  statusOk,
			},
			{
				Key:   conventions.OtelStatusDescription,
				VType: model.ValueType_STRING,
				VStr:  "status-ok",
			},
		},
		References: []model.SpanRef{
			{
				TraceID: traceID,
				SpanID:  model.NewSpanID(binary.BigEndian.Uint64([]byte{0xAF, 0xAE, 0xAD, 0xAC, 0xAB, 0xAA, 0xA9, 0xA8})),
				RefType: model.SpanRefType_CHILD_OF,
			},
			{
				TraceID: traceID,
				SpanID:  model.NewSpanID(binary.BigEndian.Uint64([]byte{0x1F, 0x1E, 0x1D, 0x1C, 0x1B, 0x1A, 0x19, 0x18})),
				RefType: model.SpanRefType_CHILD_OF,
			},
		},
	}
}

func BenchmarkProtoBatchToInternalTraces(b *testing.B) {
	jb := []*model.Batch{
		{
			Process: generateProtoProcess(),
			Spans: []*model.Span{
				generateProtoSpan(),
				generateProtoChildSpan(),
			},
		},
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		_, err := ProtoToTraces(jb)
		assert.NoError(b, err)
	}
}

func generateTracesTwoSpansFromTwoLibraries() ptrace.Traces {
	td := generateTracesOneEmptyResourceSpans()

	rs0 := td.ResourceSpans().At(0)
	rs0.ScopeSpans().EnsureCapacity(2)

	rs0ils0 := rs0.ScopeSpans().AppendEmpty()
	rs0ils0.Scope().SetName("library1")
	rs0ils0.Scope().SetVersion("0.42.0")
	span1 := rs0ils0.Spans().AppendEmpty()
	span1.SetTraceID(idutils.UInt64ToTraceID(0, 0))
	span1.SetSpanID(idutils.UInt64ToSpanID(0))
	span1.SetName("operation1")
	span1.SetStartTimestamp(testSpanStartTimestamp)
	span1.SetEndTimestamp(testSpanEndTimestamp)

	rs0ils1 := rs0.ScopeSpans().AppendEmpty()
	rs0ils1.Scope().SetName("library2")
	rs0ils1.Scope().SetVersion("0.42.0")
	span2 := rs0ils1.Spans().AppendEmpty()
	span2.SetTraceID(span1.TraceID())
	span2.SetSpanID(span1.SpanID())
	span2.SetName("operation2")
	span2.SetStartTimestamp(testSpanStartTimestamp)
	span2.SetEndTimestamp(testSpanEndTimestamp)

	return td
}
