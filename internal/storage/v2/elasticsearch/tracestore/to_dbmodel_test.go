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
	"sort"
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
	traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	modelSpan := ToDBModel(traces, false, nil, ".")
	assert.Len(t, modelSpan, 1)
	assert.Empty(t, modelSpan[0].Tags)
}

func TestEmptyLinkRefs(t *testing.T) {
	traces := ptrace.NewTraces()
	span := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	spanLink := span.Links().AppendEmpty()
	spanLink.Attributes().PutStr("testing-key", "testing-inputValue")
	modelSpan := ToDBModel(traces, false, nil, ".")
	assert.Len(t, modelSpan, 1)
	assert.Len(t, modelSpan[0].References, 1)
	assert.Equal(t, dbmodel.FollowsFrom, modelSpan[0].References[0].RefType)
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
	td, err := FromDBModel([]dbmodel.Span{span}, ".")
	require.NoError(t, err)
	testTraces(t, tracesStr, td)
	spans := ToDBModel(td, false, nil, ".")
	assert.Len(t, spans, 1)
	testSpans(t, spansStr, spans[0])
}

type tagDotReplacementTests struct {
	name            string
	allTagsAsObject bool
	tagKeysAsField  []string
}

func getTagDotReplacementTests() []tagDotReplacementTests {
	return []tagDotReplacementTests{
		{
			name:            "allTagsAsObject=false",
			allTagsAsObject: false,
			tagKeysAsField: []string{
				"otel.scope.name",
				"otel.scope.version",
				"peer.service",
				"peer.ipv4",
				"temperature",
				"error",
				"sdk.version.1",
			},
		},
		{
			name:            "allTagsAsObject=true",
			allTagsAsObject: true,
		},
	}
}

func TestToDbModel_Fixtures_TagDotReplacement(t *testing.T) {
	for _, test := range getTagDotReplacementTests() {
		t.Run(test.name, func(t *testing.T) {
			spanStr, err := os.ReadFile("fixtures/es_01_tagDotReplacement.json")
			require.NoError(t, err)
			traceStr := loadTraces(t, 1)
			var span dbmodel.Span
			d := json.NewDecoder(bytes.NewReader(spanStr))
			d.UseNumber()
			err = d.Decode(&span)
			require.NoError(t, err)
			td, err := FromDBModel([]dbmodel.Span{span}, "#")
			require.NoError(t, err)
			testTraces(t, traceStr, td)
			spans := ToDBModel(td, test.allTagsAsObject, test.tagKeysAsField, "#")
			testSpans(t, spanStr, spans[0])
		})
	}
}

func sortTracesResourceAndSpanAttributes(t *testing.T, td ptrace.Traces) {
	resourceSpans := td.ResourceSpans().At(0)
	resource := resourceSpans.Resource()
	oSpan := resourceSpans.ScopeSpans().At(0).Spans().At(0)
	// Map iteration is unordered, but for assertion we need trace in specific structure, therefore we need to sort the attributed
	sortTracesAttributes(t, oSpan.Attributes())
	sortTracesAttributes(t, resource.Attributes())
}

func sortSpanTags(span *dbmodel.Span) {
	sort.Slice(span.Tags, func(i, j int) bool {
		return span.Tags[i].Key < span.Tags[j].Key
	})
	process := &span.Process
	sort.Slice(process.Tags, func(i, j int) bool {
		return process.Tags[i].Key < process.Tags[j].Key
	})
	span.Process = *process
}

func sortTracesAttributes(t *testing.T, p pcommon.Map) {
	keys := make([]string, 0)
	p.Range(func(k string, _ pcommon.Value) bool {
		keys = append(keys, k)
		return true
	})
	sort.Strings(keys)
	sortedMap := pcommon.NewMap()
	for _, k := range keys {
		v, ok := p.Get(k)
		require.True(t, ok)
		switch v.Type() {
		case pcommon.ValueTypeStr:
			sortedMap.PutStr(k, v.Str())
		case pcommon.ValueTypeInt:
			sortedMap.PutInt(k, v.Int())
		case pcommon.ValueTypeDouble:
			sortedMap.PutDouble(k, v.Double())
		case pcommon.ValueTypeBytes:
			sortedMap.PutEmptyBytes(k).FromRaw(v.Bytes().AsRaw())
		case pcommon.ValueTypeBool:
			sortedMap.PutBool(k, v.Bool())
		default:
			sortedMap.PutEmpty(k)
		}
	}
	sortedMap.CopyTo(p)
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
	sortTracesResourceAndSpanAttributes(t, actualTraces)
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
	sortSpanTags(&actualSpan)
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
		batches := ToDBModel(td, false, nil, ".")
		assert.NotEmpty(b, batches)
	}
}
