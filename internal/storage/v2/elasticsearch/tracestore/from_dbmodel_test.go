// Copyright (c) 2025 The Jaeger Authors.
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Code originally copied from https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/e49500a9b68447cbbe237fa29526ba99e4963f39/pkg/translator/jaeger/jaegerproto_to_traces_test.go

package tracestore

import (
	"encoding/hex"
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
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
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
	trace, err := FromDBModel([]*dbmodel.Span{})
	require.NoError(t, err)
	assert.Equal(t, 0, trace.ResourceSpans().Len())
}

func TestEmptySpansAndProcess(t *testing.T) {
	trace, err := FromDBModel([]*dbmodel.Span{})
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

func Test_setSpansFromDbSpans_EmptyOrNilSpans(t *testing.T) {
	tests := []struct {
		name  string
		spans []*dbmodel.Span
	}{
		{
			name:  "nil spans",
			spans: []*dbmodel.Span{nil},
		},
		{
			name:  "empty spans",
			spans: []*dbmodel.Span{new(dbmodel.Span)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			traceData := ptrace.NewTraces()
			rss := traceData.ResourceSpans()
			err := setSpansFromDbSpans(tt.spans, rss)
			require.NoError(t, err)
			assert.Equal(t, 0, rss.Len())
		})
	}
}

func Test_setAttributesFromDbTags(t *testing.T) {
	traceData := ptrace.NewTraces()
	rss := traceData.ResourceSpans().AppendEmpty().Resource().Attributes()
	kv := []dbmodel.KeyValue{{
		Key:  "testing-key",
		Type: dbmodel.ValueType("wrong-type"),
	}}
	setAttributesFromDbTags(kv, rss)
	testingKey, testingKeyFound := rss.Get("testing-key")
	assert.True(t, testingKeyFound)
	assert.Equal(t, "Got non string value for the key testing-key", testingKey.AsString())
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
	setSpanEventsFromDbSpanLogs(logs, span.Events())
	for i := 0; i < len(logs); i++ {
		assert.Equal(t, testSpanEventTime, span.Events().At(i).Timestamp().AsTime())
	}
	assert.Equal(t, 1, span.Events().At(2).Attributes().Len())
	assert.Empty(t, span.Events().At(2).Name())
}

func TestSetAttributesFromDbTags(t *testing.T) {
	tags := []dbmodel.KeyValue{
		{
			Key:   "bool-val",
			Type:  dbmodel.BoolType,
			Value: "true",
		},
		{
			Key:   "int-val",
			Type:  dbmodel.Int64Type,
			Value: "123",
		},
		{
			Key:   "string-val",
			Type:  dbmodel.StringType,
			Value: "abc",
		},
		{
			Key:   "double-val",
			Type:  dbmodel.Float64Type,
			Value: "1.23",
		},
		{
			Key:   "binary-val",
			Type:  dbmodel.BinaryType,
			Value: hex.EncodeToString([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x64, 0x7D, 0x98}),
		},
	}

	expected := pcommon.NewMap()
	expected.PutBool("bool-val", true)
	expected.PutInt("int-val", 123)
	expected.PutStr("string-val", "abc")
	expected.PutDouble("double-val", 1.23)
	expected.PutEmptyBytes("binary-val").FromRaw([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x64, 0x7D, 0x98})

	got := pcommon.NewMap()
	setAttributesFromDbTags(tags, got)

	require.EqualValues(t, expected, got)
}

func TestFromDBModel(t *testing.T) {
	tests := []struct {
		name string
		jb   []*dbmodel.Span
		td   ptrace.Traces
	}{
		{
			name: "empty",
			jb:   []*dbmodel.Span{},
			td:   ptrace.NewTraces(),
		},
		{
			name: "two-spans-child-parent",
			jb: []*dbmodel.Span{
				generateProtoSpan(),
				generateProtoChildSpan(),
			},
			td: generateTracesWithDifferentResourceTwoSpansChildParent(),
		},
		{
			name: "two-spans-with-follower",
			jb: []*dbmodel.Span{
				generateProtoSpan(),
				generateProtoFollowerSpan(),
			},
			td: generateTracesWithDifferentResourceTwoSpansWithFollower(),
		},
		{
			name: "a-spans-with-two-parent",
			jb: []*dbmodel.Span{
				generateProtoSpan(),
				generateProtoFollowerSpan(),
				generateProtoTwoParentsSpan(),
			},
			td: generateTracesWithDifferentResourceSpanWithTwoParents(),
		},
		{
			name: "no-error-from-server-span-with-4xx-http-code",
			jb: []*dbmodel.Span{
				{
					StartTime: model.TimeAsEpochMicroseconds(testSpanStartTime),
					Duration:  model.DurationAsMicroseconds(testSpanEndTime.Sub(testSpanStartTime)),
					Tags: []dbmodel.KeyValue{
						{
							Key:   model.SpanKindKey,
							Type:  dbmodel.StringType,
							Value: string(model.SpanKindServer),
						},
						{
							Key:   conventions.AttributeHTTPStatusCode,
							Type:  dbmodel.StringType,
							Value: "404",
						},
					},
					Process: dbmodel.Process{
						ServiceName: noServiceName,
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
			td, err := FromDBModel(test.jb)
			require.NoError(t, err)
			assert.EqualValues(t, test.td, td)
		})
	}
}

func TestFromDBModelForTracesWithTwoLibraries(t *testing.T) {
	jb := []*dbmodel.Span{
		{
			StartTime:     model.TimeAsEpochMicroseconds(testSpanStartTime),
			Duration:      model.DurationAsMicroseconds(testSpanEndTime.Sub(testSpanStartTime)),
			OperationName: "operation2",
			Tags: []dbmodel.KeyValue{
				{
					Key:   conventions.AttributeOtelScopeName,
					Type:  dbmodel.StringType,
					Value: "library2",
				}, {
					Key:   conventions.AttributeOtelScopeVersion,
					Type:  dbmodel.StringType,
					Value: "0.42.0",
				},
			},
		},
		{
			TraceID:       dbmodel.TraceID("0000000000000000"),
			StartTime:     model.TimeAsEpochMicroseconds(testSpanStartTime),
			Duration:      model.DurationAsMicroseconds(testSpanEndTime.Sub(testSpanStartTime)),
			OperationName: "operation1",
			Tags: []dbmodel.KeyValue{
				{
					Key:   conventions.AttributeOtelScopeName,
					Type:  dbmodel.StringType,
					Value: "library1",
				}, {
					Key:   conventions.AttributeOtelScopeVersion,
					Type:  dbmodel.StringType,
					Value: "0.42.0",
				},
			},
		},
	}
	expected := generateTracesTwoSpansFromTwoLibraries()
	library1Span := expected.ResourceSpans().At(0).ScopeSpans().At(0)
	library2Span := expected.ResourceSpans().At(1).ScopeSpans().At(0)

	actual, err := FromDBModel(jb)
	require.NoError(t, err)

	assert.Equal(t, 2, actual.ResourceSpans().Len())
	assert.Equal(t, 1, actual.ResourceSpans().At(0).ScopeSpans().Len())

	ils0 := actual.ResourceSpans().At(0).ScopeSpans().At(0)
	ils1 := actual.ResourceSpans().At(1).ScopeSpans().At(0)
	if ils0.Scope().Name() == "library1" {
		assert.EqualValues(t, library1Span, ils0)
		assert.EqualValues(t, library2Span, ils1)
	} else {
		assert.EqualValues(t, library1Span, ils1)
		assert.EqualValues(t, library2Span, ils0)
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
			setSpanStatus(attrs, span)
			assert.EqualValues(t, test.status, status)
			assert.Equal(t, test.attrsModifiedLen, attrs.Len())
		})
	}
}

func TestFromDBModelToInternalTraces(t *testing.T) {
	batches := []*dbmodel.Span{
		generateProtoSpan(),
		generateProtoSpan(),
		generateProtoChildSpan(),
	}

	expected := generateTracesOneSpanNoResource()
	resource := generateTracesResourceOnly().ResourceSpans().At(0).Resource()
	resource.CopyTo(expected.ResourceSpans().At(0).Resource())
	span := expected.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	resource.CopyTo(expected.ResourceSpans().At(1).Resource())
	expected.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).CopyTo(span)
	tgt := expected.ResourceSpans().AppendEmpty()
	twoSpans := generateTracesWithDifferentResourceTwoSpansChildParent().ResourceSpans().At(0)
	twoSpans.CopyTo(tgt)

	got, err := FromDBModel(batches)

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
	span.SetTraceID([16]byte{0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8, 0xF9, 0xFA, 0xFB, 0xFC, 0xFD, 0xFE, 0xFF, 0x80})
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

func generateProtoSpan() *dbmodel.Span {
	spanId := [8]byte{0xAF, 0xAE, 0xAD, 0xAC, 0xAB, 0xAA, 0xA9, 0xA8}
	traceId := [16]byte{0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8, 0xF9, 0xFA, 0xFB, 0xFC, 0xFD, 0xFE, 0xFF, 0x80}
	return &dbmodel.Span{
		TraceID:       getDbTraceIdFromByteArray(traceId),
		SpanID:        getDbSpanIdFromByteArray(spanId),
		OperationName: "operationA",
		StartTime:     model.TimeAsEpochMicroseconds(testSpanStartTime),
		Duration:      model.DurationAsMicroseconds(testSpanEndTime.Sub(testSpanStartTime)),
		Logs: []dbmodel.Log{
			{
				Timestamp: model.TimeAsEpochMicroseconds(testSpanEventTime),
				Fields: []dbmodel.KeyValue{
					{
						Key:   eventNameAttr,
						Type:  dbmodel.StringType,
						Value: "event-with-attr",
					},
					{
						Key:   "span-event-attr",
						Type:  dbmodel.StringType,
						Value: "span-event-attr-val",
					},
				},
			},
			{
				Timestamp: model.TimeAsEpochMicroseconds(testSpanEventTime),
				Fields: []dbmodel.KeyValue{
					{
						Key:   "attr-int",
						Type:  dbmodel.Int64Type,
						Value: "123",
					},
				},
			},
		},
		Tags: []dbmodel.KeyValue{
			{
				Key:   model.SpanKindKey,
				Type:  dbmodel.StringType,
				Value: string(model.SpanKindClient),
			},
			{
				Key:   conventions.OtelStatusCode,
				Type:  dbmodel.StringType,
				Value: statusError,
			},
			{
				Key:   tagError,
				Value: true,
				Type:  dbmodel.BoolType,
			},
			{
				Key:   conventions.OtelStatusDescription,
				Type:  dbmodel.StringType,
				Value: "status-cancelled",
			},
		},
		Process: dbmodel.Process{
			ServiceName: noServiceName,
		},
	}
}

func generateProtoSpanWithLibraryInfo(libraryName string) *dbmodel.Span {
	span := generateProtoSpan()
	span.Tags = append([]dbmodel.KeyValue{
		{
			Key:   conventions.AttributeOtelScopeName,
			Type:  dbmodel.StringType,
			Value: libraryName,
		}, {
			Key:   conventions.AttributeOtelScopeVersion,
			Type:  dbmodel.StringType,
			Value: "0.42.0",
		},
	}, span.Tags...)

	return span
}

func getDbTraceIdFromByteArray(arr [16]byte) dbmodel.TraceID {
	return dbmodel.TraceID(hex.EncodeToString(arr[:]))
}

func getDbSpanIdFromByteArray(arr [8]byte) dbmodel.SpanID {
	return dbmodel.SpanID(hex.EncodeToString(arr[:]))
}

func generateProtoSpanWithTraceState() *dbmodel.Span {
	spanId := [8]byte{0xAF, 0xAE, 0xAD, 0xAC, 0xAB, 0xAA, 0xA9, 0xA8}
	traceId := [16]byte{0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8, 0xF9, 0xFA, 0xFB, 0xFC, 0xFD, 0xFE, 0xFF, 0x80}
	return &dbmodel.Span{
		TraceID:       getDbTraceIdFromByteArray(traceId),
		SpanID:        getDbSpanIdFromByteArray(spanId),
		OperationName: "operationA",
		StartTime:     model.TimeAsEpochMicroseconds(testSpanStartTime),
		Duration:      model.DurationAsMicroseconds(testSpanEndTime.Sub(testSpanStartTime)),
		Logs: []dbmodel.Log{
			{
				Timestamp: model.TimeAsEpochMicroseconds(testSpanEventTime),
				Fields: []dbmodel.KeyValue{
					{
						Key:   eventNameAttr,
						Type:  dbmodel.StringType,
						Value: "event-with-attr",
					},
					{
						Key:   "span-event-attr",
						Type:  dbmodel.StringType,
						Value: "span-event-attr-val",
					},
				},
			},
			{
				Timestamp: model.TimeAsEpochMicroseconds(testSpanEventTime),
				Fields: []dbmodel.KeyValue{
					{
						Key:   "attr-int",
						Type:  dbmodel.Int64Type,
						Value: "123",
					},
				},
			},
		},
		Tags: []dbmodel.KeyValue{
			{
				Key:   model.SpanKindKey,
				Type:  dbmodel.StringType,
				Value: string(model.SpanKindClient),
			},
			{
				Key:   conventions.OtelStatusCode,
				Type:  dbmodel.StringType,
				Value: statusError,
			},
			{
				Key:   tagError,
				Value: true,
				Type:  dbmodel.BoolType,
			},
			{
				Key:   conventions.OtelStatusDescription,
				Type:  dbmodel.StringType,
				Value: "status-cancelled",
			},
			{
				Key:   tagW3CTraceState,
				Type:  dbmodel.StringType,
				Value: "lasterror=f39cd56cc44274fd5abd07ef1164246d10ce2955",
			},
		},
		Process: dbmodel.Process{
			ServiceName: noServiceName,
		},
	}
}

func generateTracesTwoSpansChildParent() ptrace.Traces {
	td := generateTracesOneSpanNoResource()
	spans := td.ResourceSpans().At(0).ScopeSpans().At(0).Spans()

	span := spans.AppendEmpty()
	setChildSpan(span, spans.At(0))
	return td
}

func generateTracesWithDifferentResourceTwoSpansChildParent() ptrace.Traces {
	td := generateTracesOneSpanNoResource()
	parentSpan := td.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	spans := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans()
	span := spans.AppendEmpty()
	setChildSpan(span, parentSpan)
	return td
}

func setChildSpan(span, parentSpan ptrace.Span) {
	span.SetName("operationB")
	span.SetSpanID([8]byte{0x1F, 0x1E, 0x1D, 0x1C, 0x1B, 0x1A, 0x19, 0x18})
	span.SetParentSpanID(parentSpan.SpanID())
	span.SetKind(ptrace.SpanKindServer)
	span.SetTraceID(parentSpan.TraceID())
	span.SetStartTimestamp(parentSpan.StartTimestamp())
	span.SetEndTimestamp(parentSpan.EndTimestamp())
	span.Status().SetCode(ptrace.StatusCodeUnset)
	span.Attributes().PutInt(conventions.AttributeHTTPStatusCode, 404)
}

func generateProtoChildSpan() *dbmodel.Span {
	traceID := getDbTraceIdFromByteArray([16]byte{0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8, 0xF9, 0xFA, 0xFB, 0xFC, 0xFD, 0xFE, 0xFF, 0x80})
	return &dbmodel.Span{
		TraceID:       traceID,
		SpanID:        getDbSpanIdFromByteArray([8]byte{0x1F, 0x1E, 0x1D, 0x1C, 0x1B, 0x1A, 0x19, 0x18}),
		OperationName: "operationB",
		StartTime:     model.TimeAsEpochMicroseconds(testSpanStartTime),
		Duration:      model.DurationAsMicroseconds(testSpanEndTime.Sub(testSpanStartTime)),
		Tags: []dbmodel.KeyValue{
			{
				Key:   conventions.AttributeHTTPStatusCode,
				Type:  dbmodel.Int64Type,
				Value: "404",
			},
			{
				Key:   model.SpanKindKey,
				Type:  dbmodel.StringType,
				Value: string(model.SpanKindServer),
			},
		},
		References: []dbmodel.Reference{
			{
				TraceID: traceID,
				SpanID:  getDbSpanIdFromByteArray([8]byte{0xAF, 0xAE, 0xAD, 0xAC, 0xAB, 0xAA, 0xA9, 0xA8}),
				RefType: dbmodel.ChildOf,
			},
		},
		Process: dbmodel.Process{
			ServiceName: noServiceName,
		},
	}
}

func generateTracesTwoSpansWithFollower() ptrace.Traces {
	td := generateTracesOneSpanNoResource()
	spans := td.ResourceSpans().At(0).ScopeSpans().At(0).Spans()

	span := spans.AppendEmpty()
	setFollowFromSpan(span, spans.At(0))
	return td
}

func generateTracesWithDifferentResourceTwoSpansWithFollower() ptrace.Traces {
	td := generateTracesOneSpanNoResource()
	followFromSpan := td.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	spans := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans()
	span := spans.AppendEmpty()
	setFollowFromSpan(span, followFromSpan)
	return td
}

func setFollowFromSpan(span, followFromSpan ptrace.Span) {
	span.SetName("operationC")
	span.SetSpanID([8]byte{0x1F, 0x1E, 0x1D, 0x1C, 0x1B, 0x1A, 0x19, 0x18})
	span.SetTraceID(followFromSpan.TraceID())
	span.SetParentSpanID(followFromSpan.SpanID())
	span.SetStartTimestamp(followFromSpan.EndTimestamp())
	span.SetEndTimestamp(followFromSpan.EndTimestamp() + 1000000)
	span.SetKind(ptrace.SpanKindConsumer)
	span.Status().SetCode(ptrace.StatusCodeOk)
	span.Status().SetMessage("status-ok")
	link := span.Links().AppendEmpty()
	link.SetTraceID(span.TraceID())
	link.SetSpanID(followFromSpan.SpanID())
	link.Attributes().PutStr(
		conventions.AttributeOpentracingRefType,
		conventions.AttributeOpentracingRefTypeFollowsFrom,
	)
}

func generateProtoFollowerSpan() *dbmodel.Span {
	traceID := getDbTraceIdFromByteArray([16]byte{0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8, 0xF9, 0xFA, 0xFB, 0xFC, 0xFD, 0xFE, 0xFF, 0x80})
	return &dbmodel.Span{
		TraceID:       traceID,
		SpanID:        getDbSpanIdFromByteArray([8]byte{0x1F, 0x1E, 0x1D, 0x1C, 0x1B, 0x1A, 0x19, 0x18}),
		OperationName: "operationC",
		StartTime:     model.TimeAsEpochMicroseconds(testSpanEndTime),
		Duration:      model.DurationAsMicroseconds(time.Millisecond),
		Tags: []dbmodel.KeyValue{
			{
				Key:   model.SpanKindKey,
				Type:  dbmodel.StringType,
				Value: string(model.SpanKindConsumer),
			},
			{
				Key:   conventions.OtelStatusCode,
				Type:  dbmodel.StringType,
				Value: statusOk,
			},
			{
				Key:   conventions.OtelStatusDescription,
				Type:  dbmodel.StringType,
				Value: "status-ok",
			},
		},
		References: []dbmodel.Reference{
			{
				TraceID: traceID,
				SpanID:  getDbSpanIdFromByteArray([8]byte{0xAF, 0xAE, 0xAD, 0xAC, 0xAB, 0xAA, 0xA9, 0xA8}),
				RefType: dbmodel.FollowsFrom,
			},
		},
		Process: dbmodel.Process{
			ServiceName: noServiceName,
		},
	}
}

func generateTracesSpanWithTwoParents() ptrace.Traces {
	td := generateTracesTwoSpansWithFollower()
	spans := td.ResourceSpans().At(0).ScopeSpans().At(0).Spans()
	parent := spans.At(0)
	parent2 := spans.At(1)
	span := spans.AppendEmpty()
	setSpanWithTwoParents(span, parent, parent2)
	return td
}

func generateTracesWithDifferentResourceSpanWithTwoParents() ptrace.Traces {
	td := generateTracesWithDifferentResourceTwoSpansWithFollower()
	parent1 := td.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	parent2 := td.ResourceSpans().At(1).ScopeSpans().At(0).Spans().At(0)
	span := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	setSpanWithTwoParents(span, parent1, parent2)
	return td
}

func setSpanWithTwoParents(span, parent, parent2 ptrace.Span) {
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
}

func generateProtoTwoParentsSpan() *dbmodel.Span {
	traceID := getDbTraceIdFromByteArray([16]byte{0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8, 0xF9, 0xFA, 0xFB, 0xFC, 0xFD, 0xFE, 0xFF, 0x80})
	return &dbmodel.Span{
		TraceID:       traceID,
		SpanID:        getDbSpanIdFromByteArray([8]byte{0x1F, 0x1E, 0x1D, 0x1C, 0x1B, 0x1A, 0x19, 0x20}),
		OperationName: "operationD",
		StartTime:     model.TimeAsEpochMicroseconds(testSpanStartTime),
		Duration:      model.DurationAsMicroseconds(testSpanEndTime.Sub(testSpanStartTime)),
		Tags: []dbmodel.KeyValue{
			{
				Key:   model.SpanKindKey,
				Type:  dbmodel.StringType,
				Value: string(model.SpanKindConsumer),
			},
			{
				Key:   conventions.OtelStatusCode,
				Type:  dbmodel.StringType,
				Value: statusOk,
			},
			{
				Key:   conventions.OtelStatusDescription,
				Type:  dbmodel.StringType,
				Value: "status-ok",
			},
		},
		References: []dbmodel.Reference{
			{
				TraceID: traceID,
				SpanID:  getDbSpanIdFromByteArray([8]byte{0xAF, 0xAE, 0xAD, 0xAC, 0xAB, 0xAA, 0xA9, 0xA8}),
				RefType: dbmodel.ChildOf,
			},
			{
				TraceID: traceID,
				SpanID:  getDbSpanIdFromByteArray([8]byte{0x1F, 0x1E, 0x1D, 0x1C, 0x1B, 0x1A, 0x19, 0x18}),
				RefType: dbmodel.ChildOf,
			},
		},
		Process: dbmodel.Process{
			ServiceName: noServiceName,
		},
	}
}

func BenchmarkProtoBatchToInternalTraces(b *testing.B) {
	jb := []*dbmodel.Span{
		generateProtoSpan(),
		generateProtoChildSpan(),
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		_, err := FromDBModel(jb)
		assert.NoError(b, err)
	}
}

func generateTracesTwoSpansFromTwoLibraries() ptrace.Traces {
	td := generateTracesOneEmptyResourceSpans()

	rs0 := td.ResourceSpans().At(0)
	rs0.ScopeSpans().EnsureCapacity(1)

	rs0ils0 := rs0.ScopeSpans().AppendEmpty()
	rs0ils0.Scope().SetName("library1")
	rs0ils0.Scope().SetVersion("0.42.0")
	span1 := rs0ils0.Spans().AppendEmpty()
	span1.SetTraceID(idutils.UInt64ToTraceID(0, 0))
	span1.SetSpanID(idutils.UInt64ToSpanID(0))
	span1.SetName("operation1")
	span1.SetStartTimestamp(testSpanStartTimestamp)
	span1.SetEndTimestamp(testSpanEndTimestamp)

	rs0ils1 := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()
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
