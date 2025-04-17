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
	conventions "go.opentelemetry.io/collector/semconv/v1.9.0"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
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
				Key:   conventions.OtelStatusCode,
				Type:  dbmodel.StringType,
				Value: statusOk,
			},
		},

		{
			name: "error",
			code: ptrace.StatusCodeError,
			tag: dbmodel.KeyValue{
				Key:   tagError,
				Type:  dbmodel.BoolType,
				Value: true,
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
	spanLink.Attributes().PutStr("testing-key", "testing-inputValue")
	modelSpan := spanToDbSpan(span, spanScope, dbmodel.Process{})
	assert.Len(t, modelSpan.References, 1)
	assert.Equal(t, dbmodel.FollowsFrom, modelSpan.References[0].RefType)
}

func TestGetTagFromStatusMsg(t *testing.T) {
	_, ok := getTagFromStatusMsg("")
	assert.False(t, ok)

	got, ok := getTagFromStatusMsg("test-error")
	assert.True(t, ok)
	assert.Equal(t, dbmodel.KeyValue{
		Key:   conventions.OtelStatusDescription,
		Value: "test-error",
		Type:  dbmodel.StringType,
	}, got)
}

func Test_resourceToDbProcess(t *testing.T) {
	traces := ptrace.NewTraces()
	resourceSpans := traces.ResourceSpans().AppendEmpty()
	resource := resourceSpans.Resource()
	resource.Attributes().PutStr(conventions.AttributeServiceName, "service")
	resource.Attributes().PutStr("foo", "bar")
	process := resourceToDbProcess(resource)
	assert.Equal(t, "service", process.ServiceName)
	expected := []dbmodel.KeyValue{
		{
			Key:   "foo",
			Value: "bar",
			Type:  dbmodel.StringType,
		},
	}
	assert.Equal(t, expected, process.Tags)
}

func Test_resourceToDbProcess_WhenOnlyServiceNameIsPresent(t *testing.T) {
	traces := ptrace.NewTraces()
	spans := traces.ResourceSpans().AppendEmpty()
	spans.Resource().Attributes().PutStr(conventions.AttributeServiceName, "service")
	process := resourceToDbProcess(spans.Resource())
	assert.Equal(t, "service", process.ServiceName)
}

func Test_appendTagsFromResourceAttributes_empty_attrs(t *testing.T) {
	traces := ptrace.NewTraces()
	emptyAttrs := traces.ResourceSpans().AppendEmpty().Resource().Attributes()
	kv := appendTagsFromAttributes([]dbmodel.KeyValue{}, emptyAttrs)
	assert.Empty(t, kv)
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
				Key:   model.SpanKindKey,
				Type:  dbmodel.StringType,
				Value: string(model.SpanKindClient),
			},
			ok: true,
		},

		{
			name: "server",
			kind: ptrace.SpanKindServer,
			tag: dbmodel.KeyValue{
				Key:   model.SpanKindKey,
				Type:  dbmodel.StringType,
				Value: string(model.SpanKindServer),
			},
			ok: true,
		},

		{
			name: "producer",
			kind: ptrace.SpanKindProducer,
			tag: dbmodel.KeyValue{
				Key:   model.SpanKindKey,
				Type:  dbmodel.StringType,
				Value: string(model.SpanKindProducer),
			},
			ok: true,
		},

		{
			name: "consumer",
			kind: ptrace.SpanKindConsumer,
			tag: dbmodel.KeyValue{
				Key:   model.SpanKindKey,
				Type:  dbmodel.StringType,
				Value: string(model.SpanKindConsumer),
			},
			ok: true,
		},

		{
			name: "internal",
			kind: ptrace.SpanKindInternal,
			tag: dbmodel.KeyValue{
				Key:   model.SpanKindKey,
				Type:  dbmodel.StringType,
				Value: string(model.SpanKindInternal),
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

func TestAttributesToDbSpanTags(t *testing.T) {
	attributes := pcommon.NewMap()
	attributes.PutBool("bool-val", true)
	attributes.PutInt("int-val", 123)
	attributes.PutStr("string-val", "abc")
	attributes.PutDouble("double-val", 1.23)
	attributes.PutEmptyBytes("bytes-val").FromRaw([]byte{1, 2, 3, 4})
	attributes.PutStr(conventions.AttributeServiceName, "service-name")

	expected := []dbmodel.KeyValue{
		{
			Key:   "bool-val",
			Type:  dbmodel.BoolType,
			Value: true,
		},
		{
			Key:   "int-val",
			Type:  dbmodel.Int64Type,
			Value: int64(123),
		},
		{
			Key:   "string-val",
			Type:  dbmodel.StringType,
			Value: "abc",
		},
		{
			Key:   "double-val",
			Type:  dbmodel.Float64Type,
			Value: 1.23,
		},
		{
			Key:   "bytes-val",
			Type:  dbmodel.BinaryType,
			Value: "01020304",
		},
		{
			Key:   conventions.AttributeServiceName,
			Type:  dbmodel.StringType,
			Value: "service-name",
		},
	}

	got := appendTagsFromAttributes(make([]dbmodel.KeyValue, 0, len(expected)), attributes)
	require.Equal(t, expected, got)
}

func TestAttributesToDbSpanTags_MapType(t *testing.T) {
	attributes := pcommon.NewMap()
	attributes.PutEmptyMap("empty-map")
	got := appendTagsFromAttributes(make([]dbmodel.KeyValue, 0, 1), attributes)
	expected := []dbmodel.KeyValue{
		{
			Key:   "empty-map",
			Type:  dbmodel.StringType,
			Value: "{}",
		},
	}
	require.Equal(t, expected, got)
}

func TestToDbModel_Fixtures(t *testing.T) {
	tracesStr, spansStr := loadFixtures(t, 1)
	var span dbmodel.Span
	err := json.Unmarshal(spansStr, &span)
	require.NoError(t, err)
	td, err := FromDBModel([]dbmodel.Span{span})
	require.NoError(t, err)
	testTraces(t, tracesStr, td)
	spans := ToDBModel(td)
	assert.Len(t, spans, 1)
	testSpans(t, spansStr, spans[0])
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
	inSpans := fmt.Sprintf("fixtures/es_%02d.json", i)
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

func BenchmarkInternalTracesToDbSpans(b *testing.B) {
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
