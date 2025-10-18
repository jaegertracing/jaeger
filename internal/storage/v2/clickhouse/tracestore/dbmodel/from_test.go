// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
)

func TestFromDBModel_Fixtures(t *testing.T) {
	dbTrace := jsonToDBModel(t, "./fixtures/dbmodel.json")
	expected := jsonToPtrace(t, "./fixtures/ptrace.json")
	actual := FromDBModel(dbTrace)

	require.Equal(t, expected.ResourceSpans().Len(), actual.ResourceSpans().Len(), "ResourceSpans count mismatch")
	if actual.ResourceSpans().Len() == 0 {
		t.Fatal("Actual trace contains no ResourceSpans")
	}
	expectedResourceSpans := expected.ResourceSpans().At(0)
	actualResourceSpans := actual.ResourceSpans().At(0)

	require.Equal(t, expectedResourceSpans.ScopeSpans().Len(), actualResourceSpans.ScopeSpans().Len(), "ScopeSpans count mismatch")
	if actualResourceSpans.ScopeSpans().Len() == 0 {
		t.Fatal("Actual ResourceSpans contains no ScopeSpans")
	}
	exceptedScopeSpans := expectedResourceSpans.ScopeSpans().At(0)
	actualScopeSpans := actualResourceSpans.ScopeSpans().At(0)

	require.Equal(t, exceptedScopeSpans.Spans().Len(), actualScopeSpans.Spans().Len(), "Spans count mismatch")
	if actualScopeSpans.Spans().Len() == 0 {
		t.Fatal("Actual ScopeSpans contains no Spans")
	}
	exceptedSpan := exceptedScopeSpans.Spans().At(0)
	actualSpan := actualScopeSpans.Spans().At(0)

	t.Run("Resource", func(*testing.T) {
		// TODO: add resource attributes comparison
	})

	t.Run("Scope", func(t *testing.T) {
		actualScope := actualScopeSpans.Scope()
		expectedScope := exceptedScopeSpans.Scope()
		require.Equal(t, expectedScope.Name(), actualScope.Name(), "Scope attributes mismatch")
		require.Equal(t, expectedScope.Version(), actualScope.Version(), "Scope attributes mismatch")
	})

	t.Run("Span", func(t *testing.T) {
		require.Equal(t, exceptedSpan.StartTimestamp(), actualSpan.StartTimestamp(), "Span attributes mismatch")
		require.Equal(t, exceptedSpan.TraceID(), actualSpan.TraceID(), "Span attributes mismatch")
		require.Equal(t, exceptedSpan.SpanID(), actualSpan.SpanID(), "Span attributes mismatch")
		require.Equal(t, exceptedSpan.ParentSpanID(), actualSpan.ParentSpanID(), "Span attributes mismatch")
		require.Equal(t, exceptedSpan.TraceState(), actualSpan.TraceState(), "Span attributes mismatch")
		require.Equal(t, exceptedSpan.Name(), actualSpan.Name(), "Span attributes mismatch")
		require.Equal(t, exceptedSpan.Kind(), actualSpan.Kind(), "Span attributes mismatch")
		require.Equal(t, exceptedSpan.EndTimestamp(), actualSpan.EndTimestamp(), "Span attributes mismatch")
		require.Equal(t, exceptedSpan.Status().Code(), actualSpan.Status().Code(), "Span attributes mismatch")
		require.Equal(t, exceptedSpan.Status().Message(), actualSpan.Status().Message(), "Span attributes mismatch")
		exceptedSpan.Attributes().Range(func(k string, v pcommon.Value) bool {
			actualValue, ok := actualSpan.Attributes().Get(k)
			require.True(t, ok, "Missing attribute %s", k)
			require.Equal(t, v, actualValue, "Attribute %s mismatch", k)
			return true
		})
	})

	t.Run("Events", func(t *testing.T) {
		exceptedEvents := exceptedSpan.Events()
		actualEvents := actualSpan.Events()
		require.Equal(t, exceptedEvents.Len(), actualEvents.Len(), "Events count mismatch")
		for i := 0; i < exceptedEvents.Len(); i++ {
			exceptedEvent := exceptedEvents.At(i)
			actualEvent := actualEvents.At(i)
			require.Equal(t, exceptedEvent.Name(), actualEvent.Name(), "Event attributes mismatch")
			require.Equal(t, exceptedEvent.Timestamp(), actualEvent.Timestamp(), "Event attributes mismatch")
			exceptedEvent.Attributes().Range(func(k string, v pcommon.Value) bool {
				actualValue, ok := actualEvent.Attributes().Get(k)
				require.True(t, ok, "Missing attribute %s", k)
				require.Equal(t, v, actualValue, "Attribute %s mismatch", k)
				return true
			})
		}
	})

	t.Run("Links", func(t *testing.T) {
		exceptedLinks := exceptedSpan.Links()
		actualLinks := actualSpan.Links()
		require.Equal(t, exceptedLinks.Len(), actualLinks.Len(), "Links count mismatch")
		for i := 0; i < exceptedLinks.Len(); i++ {
			exceptedLink := exceptedLinks.At(i)
			actualLink := actualLinks.At(i)
			require.Equal(t, exceptedLink.TraceID(), actualLink.TraceID(), "Link attributes mismatch")
			require.Equal(t, exceptedLink.SpanID(), actualLink.SpanID(), "Link attributes mismatch")
			require.Equal(t, exceptedLink.TraceState(), actualLink.TraceState(), "Link attributes mismatch")
		}
	})
}

func jsonToDBModel(t *testing.T, filename string) (m Span) {
	traceBytes := readJSONBytes(t, filename)
	err := json.Unmarshal(traceBytes, &m)
	require.NoError(t, err, "Failed to read file %s", filename)
	return m
}

func jsonToPtrace(t *testing.T, filename string) (trace ptrace.Traces) {
	unMarshaler := ptrace.JSONUnmarshaler{}
	trace, err := unMarshaler.UnmarshalTraces(readJSONBytes(t, filename))
	require.NoError(t, err, "Failed to unmarshal trace with %s", filename)
	return trace
}

func TestFromDBModel_DecodeID(t *testing.T) {
	tests := []struct {
		name string
		arg  Span
		want string
	}{
		{
			name: "decode span trace id failed",
			arg: Span{
				TraceID: "0x",
			},
			want: "failed to decode trace ID: encoding/hex: invalid byte: U+0078 'x'",
		},
		{
			name: "decode span id failed",
			arg: Span{
				TraceID: "00010001000100010001000100010001",
				ID:      "0x",
			},
			want: "failed to decode span ID: encoding/hex: invalid byte: U+0078 'x'",
		},
		{
			name: "decode span parent id failed",
			arg: Span{
				TraceID:      "00010001000100010001000100010001",
				ID:           "0001000100010001",
				ParentSpanID: "0x",
			},
			want: "failed to decode parent span ID: encoding/hex: invalid byte: U+0078 'x'",
		},
		{
			name: "decode link trace id failed",
			arg: Span{
				TraceID:      "00010001000100010001000100010001",
				ID:           "0001000100010001",
				ParentSpanID: "0001000100010001",
				Links: []Link{
					{
						TraceID: "0x",
					},
				},
			},
			want: "failed to decode link trace ID: encoding/hex: invalid byte: U+0078 'x'",
		},
		{
			name: "decode link span id failed",
			arg: Span{
				TraceID:      "00010001000100010001000100010001",
				ID:           "0001000100010001",
				ParentSpanID: "0001000100010001",
				Links: []Link{
					{
						TraceID: "00010001000100010001000100010001",
						SpanID:  "0x",
					},
				},
			},
			want: "failed to decode link span ID: encoding/hex: invalid byte: U+0078 'x'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trace := FromDBModel(tt.arg)
			span := trace.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
			require.Contains(t, jptrace.GetWarnings(span), tt.want)
		})
	}
}

func TestPopulateComplexAttributes(t *testing.T) {
	tests := []struct {
		name               string
		complexAttributes  []Attribute[string]
		expectedAttributes map[string]pcommon.Value
		expectedWarnings   []string
	}{
		{
			name: "bytes attribute success",
			complexAttributes: []Attribute[string]{
				{
					Key:   "@bytes@data",
					Value: base64.StdEncoding.EncodeToString([]byte("hello world")),
				},
			},
			expectedAttributes: map[string]pcommon.Value{
				"data": func() pcommon.Value {
					val := pcommon.NewValueBytes()
					val.Bytes().FromRaw([]byte("hello world"))
					return val
				}(),
			},
			expectedWarnings: nil,
		},
		{
			name: "invalid base64 encoding",
			complexAttributes: []Attribute[string]{
				{
					Key:   "@bytes@invalid",
					Value: "invalid-base64!",
				},
			},
			expectedAttributes: map[string]pcommon.Value{},
			expectedWarnings:   []string{"failed to decode bytes attribute \"invalid\""},
		},
		{
			name: "unsupported complex attribute type",
			complexAttributes: []Attribute[string]{
				{
					Key:   "@unknown@test",
					Value: "some value",
				},
			},
			expectedAttributes: map[string]pcommon.Value{},
			expectedWarnings:   []string{"unsupported complex attribute type for key \"@unknown@test\""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			attributes := span.Attributes()

			populateComplexAttributes(attributes, tt.complexAttributes, span)

			for expectedKey, expectedValue := range tt.expectedAttributes {
				actualValue, exists := attributes.Get(expectedKey)
				require.True(t, exists, "Expected attribute %s not found", expectedKey)
				require.Equal(t, expectedValue, actualValue, "Attribute %s value mismatch", expectedKey)
			}

			actualWarnings := jptrace.GetWarnings(span)
			if tt.expectedWarnings == nil {
				require.Empty(t, actualWarnings, "Expected no warnings but got: %v", actualWarnings)
			} else {
				require.Len(t, actualWarnings, len(tt.expectedWarnings), "Warning count mismatch")
				for i, expectedWarning := range tt.expectedWarnings {
					require.Contains(t, actualWarnings[i], expectedWarning, "Warning %d mismatch", i)
				}
			}
		})
	}
}
