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
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
	conventions "github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
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

func TestLinksToDbSpanRefs(t *testing.T) {
	tests := []struct {
		name            string
		setupSpan       func() ptrace.Span
		setupLinks      func(ptrace.SpanLinkSlice)
		expectedRefs    int
		expectedRefType dbmodel.ReferenceType
		description     string
	}{
		{
			name: "empty links with follows from attribute",
			setupSpan: func() ptrace.Span {
				traces := ptrace.NewTraces()
				spans := traces.ResourceSpans().AppendEmpty()
				scopeSpans := spans.ScopeSpans().AppendEmpty()
				return scopeSpans.Spans().AppendEmpty()
			},
			setupLinks: func(links ptrace.SpanLinkSlice) {
				link := links.AppendEmpty()
				link.Attributes().PutStr("testing-key", "testing-inputValue")
				link.Attributes().PutStr(conventions.AttributeOpentracingRefType, conventions.AttributeOpentracingRefTypeFollowsFrom)
			},
			expectedRefs:    1,
			expectedRefType: dbmodel.FollowsFrom,
			description:     "Links with explicit follows-from reference type",
		},
		{
			name: "links without ref type defaults to FollowsFrom",
			setupSpan: func() ptrace.Span {
				traces := ptrace.NewTraces()
				spans := traces.ResourceSpans().AppendEmpty()
				scopeSpans := spans.ScopeSpans().AppendEmpty()
				return scopeSpans.Spans().AppendEmpty()
			},
			setupLinks: func(links ptrace.SpanLinkSlice) {
				link := links.AppendEmpty()
				link.Attributes().PutStr("testing-key", "testing-inputValue")
			},
			expectedRefs:    1,
			expectedRefType: dbmodel.FollowsFrom,
			description:     "Links without reference type should default to FollowsFrom",
		},
		{
			name: "parent reference with follows from link",
			setupSpan: func() ptrace.Span {
				traces := ptrace.NewTraces()
				resourceSpans := traces.ResourceSpans().AppendEmpty()
				scopeSpans := resourceSpans.ScopeSpans().AppendEmpty()
				span := scopeSpans.Spans().AppendEmpty()
				span.SetTraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
				span.SetSpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8})
				span.SetParentSpanID([8]byte{8, 7, 6, 5, 4, 3, 2, 1})
				return span
			},
			setupLinks: func(links ptrace.SpanLinkSlice) {
				link := links.AppendEmpty()
				link.SetTraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
				link.SetSpanID([8]byte{8, 7, 6, 5, 4, 3, 2, 1})
				link.Attributes().PutStr(conventions.AttributeOpentracingRefType, conventions.AttributeOpentracingRefTypeFollowsFrom)
			},
			expectedRefs:    1,
			expectedRefType: dbmodel.FollowsFrom,
			description:     "Parent reference should be overridden by link with follows-from type",
		},
		{
			name: "empty span no links",
			setupSpan: func() ptrace.Span {
				traces := ptrace.NewTraces()
				spans := traces.ResourceSpans().AppendEmpty()
				scopeSpans := spans.ScopeSpans().AppendEmpty()
				return scopeSpans.Spans().AppendEmpty()
			},
			setupLinks: func(_ ptrace.SpanLinkSlice) {
				// No links added
			},
			expectedRefs:    0,
			expectedRefType: dbmodel.ChildOf, // Not used in this case
			description:     "Span with no links should have no references",
		},
		{
			name: "span with client kind",
			setupSpan: func() ptrace.Span {
				traces := ptrace.NewTraces()
				spans := traces.ResourceSpans().AppendEmpty()
				scopeSpans := spans.ScopeSpans().AppendEmpty()
				span := scopeSpans.Spans().AppendEmpty()
				span.SetKind(ptrace.SpanKindClient) // This triggers spanKindTagFound = true
				return span
			},
			setupLinks: func(_ ptrace.SpanLinkSlice) {
				// No links needed for this test
			},
			expectedRefs:    0,
			expectedRefType: dbmodel.ChildOf,
			description:     "Span with client kind should have span kind tag",
		},
		{
			name: "span with unspecified kind",
			setupSpan: func() ptrace.Span {
				traces := ptrace.NewTraces()
				spans := traces.ResourceSpans().AppendEmpty()
				scopeSpans := spans.ScopeSpans().AppendEmpty()
				span := scopeSpans.Spans().AppendEmpty()
				span.SetKind(ptrace.SpanKindUnspecified) // This triggers spanKindTagFound = false
				return span
			},
			setupLinks: func(_ ptrace.SpanLinkSlice) {
				// No links needed for this test
			},
			expectedRefs:    0,
			expectedRefType: dbmodel.ChildOf,
			description:     "Span with unspecified kind should not have span kind tag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := tt.setupSpan()
			tt.setupLinks(span.Links())

			scopeSpans := ptrace.NewScopeSpans()
			modelSpan := spanToDbSpan(span, scopeSpans.Scope(), dbmodel.Process{})

			assert.Len(t, modelSpan.References, tt.expectedRefs, tt.description)
			if tt.expectedRefs > 0 {
				assert.Equal(t, tt.expectedRefType, modelSpan.References[0].RefType)
			}
		})
	}
}

func TestResourceToDbProcess(t *testing.T) {
	tests := []struct {
		name            string
		setupResource   func() pcommon.Resource
		expectedService string
		expectedTags    []dbmodel.KeyValue
		description     string
	}{
		{
			name: "resource with service name and attributes",
			setupResource: func() pcommon.Resource {
				traces := ptrace.NewTraces()
				resource := traces.ResourceSpans().AppendEmpty().Resource()
				resource.Attributes().PutStr(conventions.ServiceNameKey, "service")
				resource.Attributes().PutStr("foo", "bar")
				return resource
			},
			expectedService: "service",
			expectedTags: []dbmodel.KeyValue{
				{
					Key:   "foo",
					Value: "bar",
					Type:  dbmodel.StringType,
				},
			},
			description: "Resource with service name and additional attributes",
		},
		{
			name: "resource with only service name",
			setupResource: func() pcommon.Resource {
				traces := ptrace.NewTraces()
				resource := traces.ResourceSpans().AppendEmpty().Resource()
				resource.Attributes().PutStr(conventions.ServiceNameKey, "service")
				return resource
			},
			expectedService: "service",
			expectedTags:    []dbmodel.KeyValue{},
			description:     "Resource with only service name should have empty tags",
		},
		{
			name: "resource with no attributes",
			setupResource: func() pcommon.Resource {
				traces := ptrace.NewTraces()
				return traces.ResourceSpans().AppendEmpty().Resource()
			},
			expectedService: noServiceName,
			expectedTags:    nil,
			description:     "Resource with no attributes should use default service name",
		},
		{
			name: "resource with empty service name",
			setupResource: func() pcommon.Resource {
				traces := ptrace.NewTraces()
				resource := traces.ResourceSpans().AppendEmpty().Resource()
				resource.Attributes().PutStr(conventions.ServiceNameKey, "") // Explicitly empty string
				resource.Attributes().PutStr("foo", "bar")
				return resource
			},
			expectedService: "",
			expectedTags: []dbmodel.KeyValue{
				{
					Key:   "foo",
					Value: "bar",
					Type:  dbmodel.StringType,
				},
			},
			description: "Resource with empty service name should use empty string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := tt.setupResource()
			process := resourceToDbProcess(resource)

			assert.Equal(t, tt.expectedService, process.ServiceName, tt.description)
			assert.Equal(t, tt.expectedTags, process.Tags)
		})
	}
}

func TestAttributeConversion(t *testing.T) {
	tests := []struct {
		name      string
		setupAttr func() pcommon.Map
		expected  []dbmodel.KeyValue
	}{
		{
			name: "mixed attribute types",
			setupAttr: func() pcommon.Map {
				attributes := pcommon.NewMap()
				attributes.PutBool("bool-val", true)
				attributes.PutInt("int-val", 123)
				attributes.PutStr("string-val", "abc")
				attributes.PutDouble("double-val", 1.23)
				attributes.PutEmptyBytes("bytes-val").FromRaw([]byte{1, 2, 3, 4})
				attributes.PutStr(conventions.ServiceNameKey, "service-name")
				return attributes
			},
			expected: []dbmodel.KeyValue{
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
					Key:   conventions.ServiceNameKey,
					Type:  dbmodel.StringType,
					Value: "service-name",
				},
			},
		},
		{
			name:      "empty attributes",
			setupAttr: pcommon.NewMap,
			expected:  []dbmodel.KeyValue{},
		},
		{
			name: "map type attributes",
			setupAttr: func() pcommon.Map {
				attributes := pcommon.NewMap()
				attributes.PutEmptyMap("empty-map")
				return attributes
			},
			expected: []dbmodel.KeyValue{
				{
					Key:   "empty-map",
					Type:  dbmodel.StringType,
					Value: "{}",
				},
			},
		},
		{
			name: "slice type attributes",
			setupAttr: func() pcommon.Map {
				attributes := pcommon.NewMap()
				slice := attributes.PutEmptySlice("blockers")
				slice.AppendEmpty().SetStr("2804-5")
				slice.AppendEmpty().SetStr("1234-6")
				return attributes
			},
			expected: []dbmodel.KeyValue{
				{
					Key:   "blockers",
					Type:  dbmodel.StringType,
					Value: `["2804-5","1234-6"]`,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attrs := tt.setupAttr()
			result := appendTagsFromAttributes(make([]dbmodel.KeyValue, 0, len(tt.expected)), attrs)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStatusMessageHandling(t *testing.T) {
	tests := []struct {
		name        string
		statusMsg   string
		expectFound bool
		expected    dbmodel.KeyValue
	}{
		{
			name:        "empty status message",
			statusMsg:   "",
			expectFound: false,
			expected:    dbmodel.KeyValue{},
		},
		{
			name:        "non-empty status message",
			statusMsg:   "test-error",
			expectFound: true,
			expected: dbmodel.KeyValue{
				Key:   conventions.OtelStatusDescription,
				Value: "test-error",
				Type:  dbmodel.StringType,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := getTagFromStatusMsg(tt.statusMsg)
			assert.Equal(t, tt.expectFound, ok)
			if tt.expectFound {
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}

func TestRefTypeFromLink(t *testing.T) {
	tests := []struct {
		name      string
		setupLink func() ptrace.SpanLink
		expected  dbmodel.ReferenceType
	}{
		{
			name: "link with child_of reference type",
			setupLink: func() ptrace.SpanLink {
				link := ptrace.NewSpanLink()
				link.Attributes().PutStr(conventions.AttributeOpentracingRefType, conventions.AttributeOpentracingRefTypeChildOf)
				return link
			},
			expected: dbmodel.ChildOf,
		},
		{
			name: "link with follows_from reference type",
			setupLink: func() ptrace.SpanLink {
				link := ptrace.NewSpanLink()
				link.Attributes().PutStr(conventions.AttributeOpentracingRefType, conventions.AttributeOpentracingRefTypeFollowsFrom)
				return link
			},
			expected: dbmodel.FollowsFrom,
		},
		{
			name:      "link without reference type defaults to follows_from",
			setupLink: ptrace.NewSpanLink,

			expected: dbmodel.FollowsFrom,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			link := tt.setupLink()
			result := refTypeFromLink(link)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTraceStateHandling(t *testing.T) {
	tests := []struct {
		name        string
		traceState  string
		expectFound bool
		expected    []dbmodel.KeyValue
	}{
		{
			name:        "empty trace state",
			traceState:  "",
			expectFound: false,
			expected:    nil,
		},
		{
			name:        "non-empty trace state",
			traceState:  "key1=value1,key2=value2",
			expectFound: true,
			expected: []dbmodel.KeyValue{
				{
					Key:   tagW3CTraceState,
					Value: "key1=value1,key2=value2",
					Type:  dbmodel.StringType,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyValues, exists := getTagsFromTraceState(tt.traceState)
			assert.Equal(t, tt.expectFound, exists)
			assert.Equal(t, tt.expected, keyValues)
		})
	}
}

func TestReferenceTypeConversion(t *testing.T) {
	tests := []struct {
		name     string
		refType  string
		expected dbmodel.ReferenceType
	}{
		{
			name:     "child of reference",
			refType:  conventions.AttributeOpentracingRefTypeChildOf,
			expected: dbmodel.ChildOf,
		},
		{
			name:     "follows from reference",
			refType:  conventions.AttributeOpentracingRefTypeFollowsFrom,
			expected: dbmodel.FollowsFrom,
		},
		{
			name:     "unknown reference type defaults to follows from",
			refType:  "any other string",
			expected: dbmodel.FollowsFrom,
		},
		{
			name:     "empty reference type defaults to follows from",
			refType:  "",
			expected: dbmodel.FollowsFrom,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := strToDbSpanRefType(tt.refType)
			assert.Equal(t, tt.expected, result)
		})
	}
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
				spans := traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
				spanScope := traces.ResourceSpans().At(0).ScopeSpans().At(0).Scope()
				modelSpan := spanToDbSpan(spans, spanScope, dbmodel.Process{})
				return len(modelSpan.Tags) == 0
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
				resourceSpans := traces.ResourceSpans().At(0)
				dbSpans := resourceSpansToDbSpans(resourceSpans)
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
				return dbSpans == nil
			},
			description: "Traces with no resource spans should return nil",
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

func TestDbSpanTagsWithStatusAndTraceState(t *testing.T) {
	tests := []struct {
		name            string
		setupSpan       func() ptrace.Span
		expectedTagKeys []string
		description     string
	}{
		{
			name: "span with status message and trace state",
			setupSpan: func() ptrace.Span {
				span := ptrace.NewSpan()
				span.Status().SetMessage("test-error")
				span.TraceState().FromRaw("key1=value1,key2=value2")
				return span
			},
			expectedTagKeys: []string{conventions.OtelStatusDescription, tagW3CTraceState},
			description:     "Span with both status message and trace state should have both tags",
		},
		{
			name: "span with only status message",
			setupSpan: func() ptrace.Span {
				span := ptrace.NewSpan()
				span.Status().SetMessage("test-error")
				return span
			},
			expectedTagKeys: []string{conventions.OtelStatusDescription},
			description:     "Span with only status message should have only status tag",
		},
		{
			name: "span with only trace state",
			setupSpan: func() ptrace.Span {
				span := ptrace.NewSpan()
				span.TraceState().FromRaw("key1=value1,key2=value2")
				return span
			},
			expectedTagKeys: []string{tagW3CTraceState},
			description:     "Span with only trace state should have only trace state tag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := tt.setupSpan()
			tags := getDbSpanTags(span, pcommon.NewInstrumentationScope())

			assert.Len(t, tags, len(tt.expectedTagKeys), tt.description)
			for i, expectedKey := range tt.expectedTagKeys {
				assert.Equal(t, expectedKey, tags[i].Key)
			}
		})
	}
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

func TestAttributeToDbTag_DefaultCase(t *testing.T) {
	attr := pcommon.NewValueEmpty()

	tag := attributeToDbTag("test-key", attr)

	assert.Equal(t, "test-key", tag.Key)
	assert.Equal(t, dbmodel.StringType, tag.Type)
	assert.Nil(t, tag.Value)
}

func TestGetTagFromStatusCode_DefaultCase(t *testing.T) {
	tag, shouldInclude := getTagFromStatusCode(ptrace.StatusCodeUnset)

	assert.False(t, shouldInclude)
	assert.Equal(t, dbmodel.KeyValue{}, tag)
}
