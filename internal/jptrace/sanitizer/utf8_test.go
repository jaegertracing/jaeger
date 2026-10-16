// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/xpdata/entity"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

func invalidUTF8() string {
	s, _ := hex.DecodeString("fefeffff")
	return string(s)
}

func getBytesValueFromString(s string) pcommon.Value {
	b := pcommon.NewValueBytes()
	b.Bytes().Append([]byte(s)...)
	return b
}

var utf8EncodingTests = []struct {
	name          string
	key           string
	value         string
	expectedKey   string
	expectedValue pcommon.Value
}{
	{
		name:          "valid key + valid value",
		key:           "key",
		value:         "value",
		expectedKey:   "key",
		expectedValue: pcommon.NewValueStr("value"),
	},
	{
		name:          "invalid key + valid value",
		key:           invalidUTF8(),
		value:         "value",
		expectedKey:   "invalid-tag-key-1",
		expectedValue: getBytesValueFromString(invalidUTF8() + ":value"),
	},
	{
		name:          "valid key + invalid value",
		key:           "key",
		value:         invalidUTF8(),
		expectedKey:   "key",
		expectedValue: getBytesValueFromString(invalidUTF8()),
	},
	{
		name:          "invalid key + invalid value",
		key:           invalidUTF8(),
		value:         invalidUTF8(),
		expectedKey:   "invalid-tag-key-1",
		expectedValue: getBytesValueFromString(fmt.Sprintf("%s:%s", invalidUTF8(), invalidUTF8())),
	},
}

func TestUTF8Sanitizer_SanitizesResourceSpanAttributes(t *testing.T) {
	tests := utf8EncodingTests
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			traces := ptrace.NewTraces()
			traces.
				ResourceSpans().
				AppendEmpty().
				Resource().
				Attributes().
				PutStr(test.key, test.value)
			sanitizer := NewUTF8Sanitizer()
			sanitized := sanitizer(traces)
			value, ok := sanitized.
				ResourceSpans().
				At(0).
				Resource().
				Attributes().
				Get(test.expectedKey)
			require.True(t, ok)
			require.Equal(t, test.expectedValue, value)
		})
	}
}

func TestUTF8Sanitizer_SanitizesScopeSpanAttributes(t *testing.T) {
	tests := utf8EncodingTests
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			traces := ptrace.NewTraces()
			traces.
				ResourceSpans().
				AppendEmpty().
				ScopeSpans().
				AppendEmpty().
				Scope().
				Attributes().
				PutStr(test.key, test.value)
			sanitizer := NewUTF8Sanitizer()
			sanitized := sanitizer(traces)
			value, ok := sanitized.
				ResourceSpans().
				At(0).
				ScopeSpans().
				At(0).
				Scope().
				Attributes().
				Get(test.expectedKey)
			require.True(t, ok)
			require.Equal(t, test.expectedValue, value)
		})
	}
}

func TestUTF8Sanitizer_SanitizesSpanAttributes(t *testing.T) {
	tests := utf8EncodingTests
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			traces := ptrace.NewTraces()
			traces.
				ResourceSpans().
				AppendEmpty().
				ScopeSpans().
				AppendEmpty().
				Spans().
				AppendEmpty().
				Attributes().
				PutStr(test.key, test.value)
			sanitizer := NewUTF8Sanitizer()
			sanitized := sanitizer(traces)
			value, ok := sanitized.
				ResourceSpans().
				At(0).
				ScopeSpans().
				At(0).
				Spans().
				At(0).
				Attributes().
				Get(test.expectedKey)
			require.True(t, ok)
			require.Equal(t, test.expectedValue, value)
		})
	}
}

func TestUTF8Sanitizer_SanitizesInvalidSpanName(t *testing.T) {
	traces := ptrace.NewTraces()
	traces.
		ResourceSpans().
		AppendEmpty().
		ScopeSpans().
		AppendEmpty().
		Spans().
		AppendEmpty().
		SetName(invalidUTF8())
	sanitizer := NewUTF8Sanitizer()
	sanitized := sanitizer(traces)
	name := sanitized.
		ResourceSpans().
		At(0).
		ScopeSpans().
		At(0).
		Spans().
		At(0).
		Name()
	require.Equal(t, "invalid-span-name", name)
}

func TestUTF8Sanitizer_DoesNotSanitizeValidSpanName(t *testing.T) {
	traces := ptrace.NewTraces()
	traces.
		ResourceSpans().
		AppendEmpty().
		ScopeSpans().
		AppendEmpty().
		Spans().
		AppendEmpty().
		SetName("name")
	sanitizer := NewUTF8Sanitizer()
	sanitized := sanitizer(traces)
	name := sanitized.
		ResourceSpans().
		At(0).
		ScopeSpans().
		At(0).
		Spans().
		At(0).
		Name()
	require.Equal(t, "name", name)
}

func TestUTF8Sanitizer_RemovesInvalidKeys(t *testing.T) {
	k1 := fmt.Sprintf("%s-%d", invalidUTF8(), 1)
	k2 := fmt.Sprintf("%s-%d", invalidUTF8(), 2)

	traces := ptrace.NewTraces()
	attributes := traces.
		ResourceSpans().
		AppendEmpty().
		Resource().
		Attributes()

	attributes.PutStr(k1, "v1")
	attributes.PutStr(k2, "v2")

	sanitizer := NewUTF8Sanitizer()
	sanitized := sanitizer(traces)
	_, ok := sanitized.
		ResourceSpans().
		At(0).
		Resource().
		Attributes().
		Get(k1)
	require.False(t, ok)

	sanitizer = NewUTF8Sanitizer()
	sanitized = sanitizer(traces)
	_, ok = sanitized.
		ResourceSpans().
		At(0).
		Resource().
		Attributes().
		Get(k2)
	require.False(t, ok)
}

func TestUTF8Sanitizer_RemovesDuplicateInvalidKeys(t *testing.T) {
	// pdata decodes repeated keys as separate entries, so the duplicate is
	// produced on the wire: two keys of equal length are swapped for the same
	// invalid bytes after marshaling.
	traces := ptrace.NewTraces()
	attributes := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty().Attributes()
	attributes.PutStr("AAAA", "v1")
	attributes.PutStr("BBBB", "v2")
	buf, err := (&ptrace.ProtoMarshaler{}).MarshalTraces(traces)
	require.NoError(t, err)
	buf = bytes.ReplaceAll(buf, []byte("AAAA"), []byte(invalidUTF8()))
	buf = bytes.ReplaceAll(buf, []byte("BBBB"), []byte(invalidUTF8()))
	decoded, err := (&ptrace.ProtoUnmarshaler{}).UnmarshalTraces(buf)
	require.NoError(t, err)
	require.Equal(t, 2, firstSpan(decoded).Attributes().Len())

	sanitized := NewUTF8Sanitizer()(decoded)

	require.NoError(t, decodeStrictly(t, sanitized))
	got := firstSpan(sanitized).Attributes()
	assert.Equal(t, 2, got.Len())
	for i, value := range []string{"v1", "v2"} {
		v, ok := got.Get(fmt.Sprintf("%s-%d", invalidTagKey, i+1))
		require.True(t, ok)
		assert.Equal(t, []byte(invalidUTF8()+":"+value), v.Bytes().AsRaw())
	}
}

func TestUTF8Sanitizer_DoesNotSanitizeNonStringAttributeValue(t *testing.T) {
	traces := ptrace.NewTraces()
	traces.
		ResourceSpans().
		AppendEmpty().
		Resource().
		Attributes().
		PutInt("key", 99)
	sanitizer := NewUTF8Sanitizer()
	sanitized := sanitizer(traces)
	value, ok := sanitized.
		ResourceSpans().
		At(0).
		Resource().
		Attributes().
		Get("key")
	require.True(t, ok)
	require.EqualValues(t, 99, value.Int())
}

func TestUTF8Sanitizer_SanitizesNonStringAttributeValueWithInvalidKey(t *testing.T) {
	traces := ptrace.NewTraces()
	traces.
		ResourceSpans().
		AppendEmpty().
		Resource().
		Attributes().
		PutInt(invalidUTF8(), 99)
	sanitizer := NewUTF8Sanitizer()
	sanitized := sanitizer(traces)
	value, ok := sanitized.
		ResourceSpans().
		At(0).
		Resource().
		Attributes().
		Get("invalid-tag-key-1")
	require.True(t, ok)
	require.Equal(t, getBytesValueFromString(invalidUTF8()+":99"), value)
}

func TestUTF8Sanitizer_SanitizesMultipleAttributesWithInvalidKeys(t *testing.T) {
	k1 := fmt.Sprintf("%s-%d", invalidUTF8(), 1)
	k2 := fmt.Sprintf("%s-%d", invalidUTF8(), 2)
	traces := ptrace.NewTraces()
	attributes := traces.
		ResourceSpans().
		AppendEmpty().
		Resource().
		Attributes()

	attributes.PutStr(k1, "v1")
	attributes.PutStr(k2, "v2")

	sanitizer := NewUTF8Sanitizer()
	sanitized := sanitizer(traces)
	got := sanitized.
		ResourceSpans().
		At(0).
		Resource().
		Attributes()
	require.Equal(t, 2, got.Len())

	expectedValues := []pcommon.Value{
		getBytesValueFromString(k1 + ":v1"),
		getBytesValueFromString(k2 + ":v2"),
	}
	value, ok := got.
		Get("invalid-tag-key-1")
	require.True(t, ok)
	require.Contains(t, expectedValues, value)
	checked := value

	value, ok = got.
		Get("invalid-tag-key-2")
	require.True(t, ok)
	require.NotEqual(t, checked, value)
	require.Contains(t, expectedValues, value)
}

// newTracesWithEveryStringField returns traces with one span in which every
// string-bearing field of the trace model is set to valid UTF-8.
func newTracesWithEveryStringField() ptrace.Traces {
	traces := ptrace.NewTraces()
	resourceSpans := traces.ResourceSpans().AppendEmpty()
	resourceSpans.SetSchemaUrl("https://opentelemetry.io/schemas/1.26.0")
	resource := resourceSpans.Resource()
	resource.Attributes().PutStr("service.name", "service")
	resource.Attributes().PutStr("service.version", "1.0.0")
	ref := entity.ResourceEntityRefs(resource).AppendEmpty()
	ref.SetSchemaUrl("https://opentelemetry.io/schemas/1.26.0")
	ref.SetType("service")
	ref.IdKeys().Append("service.name")
	ref.DescriptionKeys().Append("service.version")

	scopeSpans := resourceSpans.ScopeSpans().AppendEmpty()
	scopeSpans.SetSchemaUrl("https://opentelemetry.io/schemas/1.26.0")
	scope := scopeSpans.Scope()
	scope.SetName("scope")
	scope.SetVersion("1.0.0")
	scope.Attributes().PutStr("key", "value")

	span := scopeSpans.Spans().AppendEmpty()
	span.SetName("span")
	span.TraceState().FromRaw("key=value")
	span.Status().SetMessage("message")
	span.Attributes().PutStr("key", "value")
	nested := span.Attributes().PutEmptyMap("map")
	nested.PutStr("key", "value")
	nested.PutEmptySlice("slice").AppendEmpty().SetEmptyMap().PutStr("key", "value")

	event := span.Events().AppendEmpty()
	event.SetName("event")
	event.Attributes().PutStr("key", "value")

	link := span.Links().AppendEmpty()
	link.TraceState().FromRaw("key=value")
	link.Attributes().PutStr("key", "value")
	return traces
}

// decodeStrictly decodes traces the way a protobuf-go based OTLP receiver does,
// which rejects invalid UTF-8 in any string field.
func decodeStrictly(t *testing.T, traces ptrace.Traces) error {
	buf, err := (&ptrace.ProtoMarshaler{}).MarshalTraces(traces)
	require.NoError(t, err)
	return proto.Unmarshal(buf, &tracepb.TracesData{})
}

func firstResourceSpans(traces ptrace.Traces) ptrace.ResourceSpans {
	return traces.ResourceSpans().At(0)
}

func firstScopeSpans(traces ptrace.Traces) ptrace.ScopeSpans {
	return firstResourceSpans(traces).ScopeSpans().At(0)
}

func firstSpan(traces ptrace.Traces) ptrace.Span {
	return firstScopeSpans(traces).Spans().At(0)
}

func assertInvalidBytesAttribute(t *testing.T, attributes pcommon.Map, key string) {
	t.Helper()
	value, ok := attributes.Get(key)
	require.True(t, ok, "attribute %q is missing", key)
	assert.Equal(t, getBytesValueFromString(invalidUTF8()), value)
}

func TestUTF8Sanitizer_SanitizesEveryStringField(t *testing.T) {
	tests := []struct {
		name   string
		poison func(ptrace.Traces)
		check  func(*testing.T, ptrace.Traces)
	}{
		{
			name: "resource schema url",
			poison: func(traces ptrace.Traces) {
				firstResourceSpans(traces).SetSchemaUrl(invalidUTF8())
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				resourceSpans := firstResourceSpans(traces)
				assert.Empty(t, resourceSpans.SchemaUrl())
				assertInvalidBytesAttribute(t, resourceSpans.Resource().Attributes(), invalidSchemaURL)
			},
		},
		{
			name: "resource attribute",
			poison: func(traces ptrace.Traces) {
				firstResourceSpans(traces).Resource().Attributes().PutStr("key", invalidUTF8())
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				assertInvalidBytesAttribute(t, firstResourceSpans(traces).Resource().Attributes(), "key")
			},
		},
		{
			name: "resource entity ref schema url",
			poison: func(traces ptrace.Traces) {
				ref := entity.ResourceEntityRefs(firstResourceSpans(traces).Resource()).AppendEmpty()
				ref.SetType("host")
				ref.SetSchemaUrl(invalidUTF8())
			},
		},
		{
			name: "resource entity ref type",
			poison: func(traces ptrace.Traces) {
				entity.ResourceEntityRefs(firstResourceSpans(traces).Resource()).AppendEmpty().SetType(invalidUTF8())
			},
		},
		{
			name: "resource entity ref id key",
			poison: func(traces ptrace.Traces) {
				ref := entity.ResourceEntityRefs(firstResourceSpans(traces).Resource()).AppendEmpty()
				ref.SetType("host")
				ref.IdKeys().Append(invalidUTF8())
			},
		},
		{
			name: "resource entity ref description key",
			poison: func(traces ptrace.Traces) {
				ref := entity.ResourceEntityRefs(firstResourceSpans(traces).Resource()).AppendEmpty()
				ref.SetType("host")
				ref.DescriptionKeys().Append(invalidUTF8())
			},
		},
		{
			name: "scope schema url",
			poison: func(traces ptrace.Traces) {
				firstScopeSpans(traces).SetSchemaUrl(invalidUTF8())
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				scopeSpans := firstScopeSpans(traces)
				assert.Empty(t, scopeSpans.SchemaUrl())
				assertInvalidBytesAttribute(t, scopeSpans.Scope().Attributes(), invalidSchemaURL)
			},
		},
		{
			name: "scope name",
			poison: func(traces ptrace.Traces) {
				firstScopeSpans(traces).Scope().SetName(invalidUTF8())
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				scope := firstScopeSpans(traces).Scope()
				assert.Equal(t, invalidScopeName, scope.Name())
				assertInvalidBytesAttribute(t, scope.Attributes(), invalidScopeName)
			},
		},
		{
			name: "scope version",
			poison: func(traces ptrace.Traces) {
				firstScopeSpans(traces).Scope().SetVersion(invalidUTF8())
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				scope := firstScopeSpans(traces).Scope()
				assert.Equal(t, invalidScopeVersion, scope.Version())
				assertInvalidBytesAttribute(t, scope.Attributes(), invalidScopeVersion)
			},
		},
		{
			name: "scope attribute",
			poison: func(traces ptrace.Traces) {
				firstScopeSpans(traces).Scope().Attributes().PutStr("key", invalidUTF8())
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				assertInvalidBytesAttribute(t, firstScopeSpans(traces).Scope().Attributes(), "key")
			},
		},
		{
			name: "span name",
			poison: func(traces ptrace.Traces) {
				firstSpan(traces).SetName(invalidUTF8())
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				span := firstSpan(traces)
				assert.Equal(t, invalidSpanName, span.Name())
				assertInvalidBytesAttribute(t, span.Attributes(), invalidSpanName)
			},
		},
		{
			name: "span trace state",
			poison: func(traces ptrace.Traces) {
				firstSpan(traces).TraceState().FromRaw(invalidUTF8())
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				span := firstSpan(traces)
				assert.Empty(t, span.TraceState().AsRaw())
				assertInvalidBytesAttribute(t, span.Attributes(), invalidTraceState)
			},
		},
		{
			name: "span status message",
			poison: func(traces ptrace.Traces) {
				firstSpan(traces).Status().SetMessage(invalidUTF8())
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				span := firstSpan(traces)
				assert.Equal(t, invalidStatusMessage, span.Status().Message())
				assertInvalidBytesAttribute(t, span.Attributes(), invalidStatusMessage)
			},
		},
		{
			name: "span attribute",
			poison: func(traces ptrace.Traces) {
				firstSpan(traces).Attributes().PutStr("key", invalidUTF8())
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				assertInvalidBytesAttribute(t, firstSpan(traces).Attributes(), "key")
			},
		},
		{
			name: "span event name",
			poison: func(traces ptrace.Traces) {
				firstSpan(traces).Events().At(0).SetName(invalidUTF8())
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				event := firstSpan(traces).Events().At(0)
				assert.Equal(t, invalidEventName, event.Name())
				assertInvalidBytesAttribute(t, event.Attributes(), invalidEventName)
			},
		},
		{
			name: "span event attribute",
			poison: func(traces ptrace.Traces) {
				firstSpan(traces).Events().At(0).Attributes().PutStr("key", invalidUTF8())
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				assertInvalidBytesAttribute(t, firstSpan(traces).Events().At(0).Attributes(), "key")
			},
		},
		{
			name: "span link trace state",
			poison: func(traces ptrace.Traces) {
				firstSpan(traces).Links().At(0).TraceState().FromRaw(invalidUTF8())
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				link := firstSpan(traces).Links().At(0)
				assert.Empty(t, link.TraceState().AsRaw())
				assertInvalidBytesAttribute(t, link.Attributes(), invalidTraceState)
			},
		},
		{
			name: "span link attribute",
			poison: func(traces ptrace.Traces) {
				firstSpan(traces).Links().At(0).Attributes().PutStr("key", invalidUTF8())
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				assertInvalidBytesAttribute(t, firstSpan(traces).Links().At(0).Attributes(), "key")
			},
		},
		{
			name: "nested attribute value",
			poison: func(traces ptrace.Traces) {
				nested, _ := firstSpan(traces).Attributes().Get("map")
				nested.Map().PutStr("key", invalidUTF8())
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				nested, _ := firstSpan(traces).Attributes().Get("map")
				assertInvalidBytesAttribute(t, nested.Map(), "key")
			},
		},
	}

	for _, test := range tests {
		for _, readOnly := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/read-only=%t", test.name, readOnly), func(t *testing.T) {
				traces := newTracesWithEveryStringField()
				test.poison(traces)
				require.Error(t, decodeStrictly(t, traces), "the test case must make the batch undecodable")
				if readOnly {
					traces.MarkReadOnly()
				}

				sanitized := NewUTF8Sanitizer()(traces)

				require.NoError(t, decodeStrictly(t, sanitized))
				refs := entity.ResourceEntityRefs(firstResourceSpans(sanitized).Resource())
				require.Equal(t, 1, refs.Len(), "only entity refs with invalid strings are dropped")
				assert.Equal(t, "service", refs.At(0).Type())
				if test.check != nil {
					test.check(t, sanitized)
				}
			})
		}
	}
}

func TestUTF8Sanitizer_SanitizesNestedValues(t *testing.T) {
	invalidBytes := []byte(invalidUTF8())
	tests := []struct {
		name     string
		value    func(pcommon.Value)
		expected any
	}{
		{
			name: "string in map",
			value: func(v pcommon.Value) {
				v.SetEmptyMap().PutStr("key", invalidUTF8())
			},
			expected: map[string]any{"key": invalidBytes},
		},
		{
			name: "string in slice",
			value: func(v pcommon.Value) {
				v.SetEmptySlice().AppendEmpty().SetStr(invalidUTF8())
			},
			expected: []any{invalidBytes},
		},
		{
			name: "key in map",
			value: func(v pcommon.Value) {
				v.SetEmptyMap().PutStr(invalidUTF8(), "value")
			},
			expected: map[string]any{"invalid-tag-key-1": []byte(invalidUTF8() + ":value")},
		},
		{
			name: "string in map in slice in map",
			value: func(v pcommon.Value) {
				v.SetEmptyMap().PutEmptySlice("slice").AppendEmpty().SetEmptyMap().PutStr("key", invalidUTF8())
			},
			expected: map[string]any{"slice": []any{map[string]any{"key": invalidBytes}}},
		},
		{
			name: "string in slice in slice",
			value: func(v pcommon.Value) {
				inner := v.SetEmptySlice().AppendEmpty().SetEmptySlice()
				inner.AppendEmpty().SetStr("valid")
				inner.AppendEmpty().SetStr(invalidUTF8())
			},
			expected: []any{[]any{"valid", invalidBytes}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			traces := ptrace.NewTraces()
			attributes := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty().Attributes()
			test.value(attributes.PutEmpty("nested"))
			require.Error(t, decodeStrictly(t, traces))

			sanitized := NewUTF8Sanitizer()(traces)

			require.NoError(t, decodeStrictly(t, sanitized))
			value, ok := firstSpan(sanitized).Attributes().Get("nested")
			require.True(t, ok)
			assert.Equal(t, test.expected, value.AsRaw())
		})
	}
}

func TestUTF8Sanitizer_KeepsValidTraces(t *testing.T) {
	traces := newTracesWithEveryStringField()
	traces.MarkReadOnly()

	sanitized := NewUTF8Sanitizer()(traces)

	assert.True(t, sanitized.IsReadOnly(), "valid read-only traces must be returned without a copy")
	expected := newTracesWithEveryStringField()
	expected.MarkReadOnly()
	assert.Equal(t, expected, sanitized)
}
