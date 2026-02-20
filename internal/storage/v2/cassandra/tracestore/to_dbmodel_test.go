// Copyright (c) 2025 The Jaeger Authors.
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Code originally copied from https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/e49500a9b68447cbbe237fa29526ba99e4963f39/pkg/translator/jaeger/traces_to_jaegerproto_test.go

package tracestore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore/dbmodel"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

func TestGetTagFromStatusCode(t *testing.T) {
	tests := []struct {
		name string
		code ptrace.StatusCode
		tag  dbmodel.KeyValue
	}{
		{
			name: "ok",
			code: ptrace.StatusCodeOk,
			tag: dbmodel.KeyValue{
				Key:         otelsemconv.OtelStatusCode,
				ValueType:   dbmodel.StringType,
				ValueString: statusOk,
			},
		},

		{
			name: "error",
			code: ptrace.StatusCodeError,
			tag: dbmodel.KeyValue{
				Key:       tagError,
				ValueType: dbmodel.BoolType,
				ValueBool: true,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := getTagFromStatusCode(test.code)
			assert.True(t, ok)
			assert.Equal(t, test.tag, got)
		})
	}
}

func TestEmptyAttributes(t *testing.T) {
	traces := ptrace.NewTraces()
	spans := traces.ResourceSpans().AppendEmpty()
	scopeSpans := spans.ScopeSpans().AppendEmpty()
	spanScope := scopeSpans.Scope()
	span := scopeSpans.Spans().AppendEmpty()
	modelSpan := spanToDbSpan(span, spanScope, dbmodel.Process{})
	assert.Empty(t, modelSpan.Tags)
}

func TestEmptyLinkRefs(t *testing.T) {
	traces := ptrace.NewTraces()
	spans := traces.ResourceSpans().AppendEmpty()
	scopeSpans := spans.ScopeSpans().AppendEmpty()
	spanScope := scopeSpans.Scope()
	span := scopeSpans.Spans().AppendEmpty()
	spanLink := span.Links().AppendEmpty()
	spanLink.Attributes().PutStr("testing-key", "testing-value")
	modelSpan := spanToDbSpan(span, spanScope, dbmodel.Process{})
	assert.Len(t, modelSpan.Refs, 1)
	assert.Equal(t, dbmodel.FollowsFrom, modelSpan.Refs[0].RefType)
}

func TestGetTagFromStatusMsg(t *testing.T) {
	_, ok := getTagFromStatusMsg("")
	assert.False(t, ok)

	got, ok := getTagFromStatusMsg("test-error")
	assert.True(t, ok)
	assert.Equal(t, dbmodel.KeyValue{
		Key:         otelsemconv.OtelStatusDescription,
		ValueString: "test-error",
		ValueType:   dbmodel.StringType,
	}, got)
}

func Test_resourceToDbProcess_WhenOnlyServiceNameIsPresent(t *testing.T) {
	traces := ptrace.NewTraces()
	spans := traces.ResourceSpans().AppendEmpty()
	spans.Resource().Attributes().PutStr(otelsemconv.ServiceNameKey, "service")
	process := resourceToDbProcess(spans.Resource())
	assert.Equal(t, "service", process.ServiceName)
}

func Test_resourceToDbProcess_DefaultServiceName(t *testing.T) {
	traces := ptrace.NewTraces()
	spans := traces.ResourceSpans().AppendEmpty()
	spans.Resource().Attributes().PutStr("some attribute", "some value")
	process := resourceToDbProcess(spans.Resource())
	assert.Equal(t, noServiceName, process.ServiceName)
}

func TestGetTagFromSpanKind(t *testing.T) {
	tests := []struct {
		name string
		kind ptrace.SpanKind
		tag  dbmodel.KeyValue
		ok   bool
	}{
		{
			name: "unspecified",
			kind: ptrace.SpanKindUnspecified,
			tag:  dbmodel.KeyValue{},
			ok:   false,
		},

		{
			name: "client",
			kind: ptrace.SpanKindClient,
			tag: dbmodel.KeyValue{
				Key:         model.SpanKindKey,
				ValueType:   dbmodel.StringType,
				ValueString: string(model.SpanKindClient),
			},
			ok: true,
		},

		{
			name: "server",
			kind: ptrace.SpanKindServer,
			tag: dbmodel.KeyValue{
				Key:         model.SpanKindKey,
				ValueType:   dbmodel.StringType,
				ValueString: string(model.SpanKindServer),
			},
			ok: true,
		},

		{
			name: "producer",
			kind: ptrace.SpanKindProducer,
			tag: dbmodel.KeyValue{
				Key:         model.SpanKindKey,
				ValueType:   dbmodel.StringType,
				ValueString: string(model.SpanKindProducer),
			},
			ok: true,
		},

		{
			name: "consumer",
			kind: ptrace.SpanKindConsumer,
			tag: dbmodel.KeyValue{
				Key:         model.SpanKindKey,
				ValueType:   dbmodel.StringType,
				ValueString: string(model.SpanKindConsumer),
			},
			ok: true,
		},

		{
			name: "internal",
			kind: ptrace.SpanKindInternal,
			tag: dbmodel.KeyValue{
				Key:         model.SpanKindKey,
				ValueType:   dbmodel.StringType,
				ValueString: string(model.SpanKindInternal),
			},
			ok: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := getTagFromSpanKind(test.kind)
			assert.Equal(t, test.ok, ok)
			assert.Equal(t, test.tag, got)
		})
	}
}

func TestAttributesToDbTags(t *testing.T) {
	attributes := pcommon.NewMap()
	attributes.PutBool("bool-val", true)
	attributes.PutInt("int-val", 123)
	attributes.PutStr("string-val", "abc")
	attributes.PutDouble("double-val", 1.23)
	attributes.PutEmptyBytes("bytes-val").FromRaw([]byte{1, 2, 3, 4})
	attributes.PutStr(otelsemconv.ServiceNameKey, "service-name")

	expected := []dbmodel.KeyValue{
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
			Key:         "bytes-val",
			ValueType:   dbmodel.BinaryType,
			ValueBinary: []byte{1, 2, 3, 4},
		},
		{
			Key:         otelsemconv.ServiceNameKey,
			ValueType:   dbmodel.StringType,
			ValueString: "service-name",
		},
	}

	got := appendTagsFromAttributes(make([]dbmodel.KeyValue, 0, len(expected)), attributes)
	require.Equal(t, expected, got)
}

func TestAttributesToJaegerProtoTags_MapType(t *testing.T) {
	attributes := pcommon.NewMap()
	attributes.PutEmptyMap("empty-map")
	got := appendTagsFromAttributes(make([]dbmodel.KeyValue, 0, 1), attributes)
	expected := []dbmodel.KeyValue{
		{
			Key:         "empty-map",
			ValueType:   dbmodel.StringType,
			ValueString: "{}",
		},
	}
	require.Equal(t, expected, got)
}

func BenchmarkInternalTracesToDbModel(b *testing.B) {
	unmarshaller := ptrace.JSONUnmarshaler{}
	data, err := os.ReadFile("fixtures/otel_traces_01.json")
	require.NoError(b, err)
	td, err := unmarshaller.UnmarshalTraces(data)
	require.NoError(b, err)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		batches := ToDBModel(td)
		assert.NotEmpty(b, batches)
	}
}

func TestToDbModel_Fixtures(t *testing.T) {
	tracesData, spansData := loadFixtures(t, 1)
	unmarshaller := ptrace.JSONUnmarshaler{}
	expectedTd, err := unmarshaller.UnmarshalTraces(tracesData)
	require.NoError(t, err)
	batches := ToDBModel(expectedTd)
	assert.Len(t, batches, 1)
	testSpans(t, spansData, batches[0])
	actualTd := FromDBModel(batches)
	testTraces(t, tracesData, actualTd)
}

func TestEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		setupTraces func() ptrace.Traces
		expected    any
		testFunc    func(ptrace.Traces) any
		description string
	}{
		{
			name: "empty span attributes",
			setupTraces: func() ptrace.Traces {
				traces := ptrace.NewTraces()
				spans := traces.ResourceSpans().AppendEmpty()
				scopeSpans := spans.ScopeSpans().AppendEmpty()
				scopeSpans.Spans().AppendEmpty()
				return traces
			},
			expected: true,
			testFunc: func(traces ptrace.Traces) any {
				modelSpan := ToDBModel(traces)[0]
				return len(modelSpan.Tags) == 0 && len(modelSpan.Process.Tags) == 0 && modelSpan.Process.ServiceName == noServiceName
			},
			description: "Empty span attributes should result in no tags",
		},
		{
			name: "resource spans with no scope spans",
			setupTraces: func() ptrace.Traces {
				traces := ptrace.NewTraces()
				traces.ResourceSpans().AppendEmpty()
				return traces
			},
			expected: true,
			testFunc: func(traces ptrace.Traces) any {
				dbSpans := ToDBModel(traces)
				return len(dbSpans) == 0
			},
			description: "Resource spans with no scope spans should return empty slice",
		},
		{
			name:        "traces with no resource spans",
			setupTraces: ptrace.NewTraces,
			expected:    true,
			testFunc: func(traces ptrace.Traces) any {
				dbSpans := ToDBModel(traces)
				return len(dbSpans) == 0
			},
			description: "Traces with no resource spans should return empty slice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			traces := tt.setupTraces()
			result := tt.testFunc(traces)
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}

func writeActualData(t *testing.T, name string, data []byte) {
	var prettyJson bytes.Buffer
	err := json.Indent(&prettyJson, data, "", "  ")
	require.NoError(t, err)
	path := "fixtures/actual_" + name + ".json"
	err = os.WriteFile(path, prettyJson.Bytes(), 0o644)
	require.NoError(t, err)
	t.Log("Saved the actual " + name + " to " + path)
}

// Loads and returns domain model and JSON model fixtures with given number i.
func loadFixtures(t *testing.T, i int) (tracesData []byte, spansData []byte) {
	tracesData = loadTraces(t, i)
	spansData = loadSpans(t, i)
	return tracesData, spansData
}

func loadTraces(t *testing.T, i int) []byte {
	inTraces := fmt.Sprintf("fixtures/otel_traces_%02d.json", i)
	tracesData, err := os.ReadFile(inTraces)
	require.NoError(t, err)
	return tracesData
}

func loadSpans(t *testing.T, i int) []byte {
	inSpans := fmt.Sprintf("fixtures/cas_%02d.json", i)
	spansData, err := os.ReadFile(inSpans)
	require.NoError(t, err)
	return spansData
}

func testTraces(t *testing.T, expectedTraces []byte, actualTraces ptrace.Traces) {
	unmarshaller := ptrace.JSONUnmarshaler{}
	expectedTd, err := unmarshaller.UnmarshalTraces(expectedTraces)
	require.NoError(t, err)
	if !assert.Equal(t, expectedTd, actualTraces) {
		marshaller := ptrace.JSONMarshaler{}
		actualTd, err := marshaller.MarshalTraces(actualTraces)
		require.NoError(t, err)
		writeActualData(t, "traces", actualTd)
	}
}

func testSpans(t *testing.T, expectedSpan []byte, actualSpan dbmodel.Span) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetIndent("", "  ")
	require.NoError(t, enc.Encode(actualSpan))
	if !assert.Equal(t, string(expectedSpan), buf.String()) {
		writeActualData(t, "spans", buf.Bytes())
	}
}
