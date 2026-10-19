// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/xpdata/entity"
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
	span := sanitized.
		ResourceSpans().
		At(0).
		ScopeSpans().
		At(0).
		Spans().
		At(0)
	require.Equal(t, invalidSpanName, span.Name())
	requireBytesAttribute(t, span.Attributes(), invalidSpanName, invalidUTF8())
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

func requireBytesAttribute(t *testing.T, attributes pcommon.Map, key, raw string) {
	t.Helper()
	value, ok := attributes.Get(key)
	require.True(t, ok)
	require.Equal(t, getBytesValueFromString(raw), value)
}

func TestUTF8Sanitizer_SanitizesRemainingStrings(t *testing.T) {
	tests := []struct {
		name   string
		poison func(ptrace.Traces)
		check  func(*testing.T, ptrace.Traces)
	}{
		{
			name: "resource schema url",
			poison: func(traces ptrace.Traces) {
				traces.ResourceSpans().AppendEmpty().SetSchemaUrl(invalidUTF8())
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				resourceSpans := traces.ResourceSpans().At(0)
				require.Equal(t, invalidSchemaURL, resourceSpans.SchemaUrl())
				requireBytesAttribute(t, resourceSpans.Resource().Attributes(), invalidSchemaURL, invalidUTF8())
			},
		},
		{
			name: "scope schema url",
			poison: func(traces ptrace.Traces) {
				traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().SetSchemaUrl(invalidUTF8())
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				scopeSpans := traces.ResourceSpans().At(0).ScopeSpans().At(0)
				require.Equal(t, invalidSchemaURL, scopeSpans.SchemaUrl())
				requireBytesAttribute(t, scopeSpans.Scope().Attributes(), invalidSchemaURL, invalidUTF8())
			},
		},
		{
			name: "scope name",
			poison: func(traces ptrace.Traces) {
				scope := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Scope()
				scope.SetName(invalidUTF8())
				scope.SetVersion("v1")
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				scope := traces.ResourceSpans().At(0).ScopeSpans().At(0).Scope()
				require.Equal(t, invalidScopeName, scope.Name())
				require.Equal(t, "v1", scope.Version())
				requireBytesAttribute(t, scope.Attributes(), invalidScopeName, invalidUTF8())
			},
		},
		{
			name: "scope version",
			poison: func(traces ptrace.Traces) {
				scope := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Scope()
				scope.SetName("scope")
				scope.SetVersion(invalidUTF8())
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				scope := traces.ResourceSpans().At(0).ScopeSpans().At(0).Scope()
				require.Equal(t, "scope", scope.Name())
				require.Equal(t, invalidScopeVersion, scope.Version())
				requireBytesAttribute(t, scope.Attributes(), invalidScopeVersion, invalidUTF8())
			},
		},
		{
			name: "span trace state",
			poison: func(traces ptrace.Traces) {
				span := appendTestSpan(traces)
				span.SetName("span")
				span.TraceState().FromRaw(invalidUTF8())
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				span := testSpan(traces)
				require.Equal(t, "span", span.Name())
				require.Equal(t, invalidTraceState, span.TraceState().AsRaw())
				requireBytesAttribute(t, span.Attributes(), invalidTraceState, invalidUTF8())
			},
		},
		{
			name: "span status message",
			poison: func(traces ptrace.Traces) {
				span := appendTestSpan(traces)
				span.SetName("span")
				span.Status().SetMessage(invalidUTF8())
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				span := testSpan(traces)
				require.Equal(t, invalidStatusMessage, span.Status().Message())
				requireBytesAttribute(t, span.Attributes(), invalidStatusMessage, invalidUTF8())
			},
		},
		{
			name: "event name",
			poison: func(traces ptrace.Traces) {
				event := appendTestSpan(traces).Events().AppendEmpty()
				event.SetName(invalidUTF8())
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				event := testSpan(traces).Events().At(0)
				require.Equal(t, invalidEventName, event.Name())
				requireBytesAttribute(t, event.Attributes(), invalidEventName, invalidUTF8())
			},
		},
		{
			name: "event attribute",
			poison: func(traces ptrace.Traces) {
				event := appendTestSpan(traces).Events().AppendEmpty()
				event.SetName("event")
				event.Attributes().PutStr("key", invalidUTF8())
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				event := testSpan(traces).Events().At(0)
				require.Equal(t, "event", event.Name())
				requireBytesAttribute(t, event.Attributes(), "key", invalidUTF8())
			},
		},
		{
			name: "link trace state",
			poison: func(traces ptrace.Traces) {
				link := appendTestSpan(traces).Links().AppendEmpty()
				link.TraceState().FromRaw(invalidUTF8())
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				link := testSpan(traces).Links().At(0)
				require.Equal(t, invalidTraceState, link.TraceState().AsRaw())
				requireBytesAttribute(t, link.Attributes(), invalidTraceState, invalidUTF8())
			},
		},
		{
			name: "link attribute",
			poison: func(traces ptrace.Traces) {
				link := appendTestSpan(traces).Links().AppendEmpty()
				link.Attributes().PutStr(invalidUTF8(), "value")
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				attributes := testSpan(traces).Links().At(0).Attributes()
				_, ok := attributes.Get(invalidUTF8())
				require.False(t, ok)
				requireBytesAttribute(t, attributes, invalidTagKey+"-1", invalidUTF8()+":value")
			},
		},
		{
			name: "resource entity ref schema url",
			poison: func(traces ptrace.Traces) {
				resourceSpans := traces.ResourceSpans().AppendEmpty()
				resourceSpans.SetSchemaUrl("https://opentelemetry.io/schemas/1.26.0")
				ref := entity.ResourceEntityRefs(resourceSpans.Resource()).AppendEmpty()
				ref.SetSchemaUrl(invalidUTF8())
				ref.SetType("service")
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				resourceSpans := traces.ResourceSpans().At(0)
				require.Equal(t, "https://opentelemetry.io/schemas/1.26.0", resourceSpans.SchemaUrl())
				refs := entity.ResourceEntityRefs(resourceSpans.Resource())
				require.Equal(t, 1, refs.Len())
				require.Equal(t, invalidEntitySchemaURL, refs.At(0).SchemaUrl())
				require.Equal(t, "service", refs.At(0).Type())
				requireBytesAttribute(t, resourceSpans.Resource().Attributes(), invalidEntitySchemaURL, invalidUTF8())
			},
		},
		{
			name: "resource entity ref type",
			poison: func(traces ptrace.Traces) {
				ref := entity.ResourceEntityRefs(traces.ResourceSpans().AppendEmpty().Resource()).AppendEmpty()
				ref.SetSchemaUrl("https://opentelemetry.io/schemas/1.26.0")
				ref.SetType(invalidUTF8())
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				resource := traces.ResourceSpans().At(0).Resource()
				refs := entity.ResourceEntityRefs(resource)
				require.Equal(t, 1, refs.Len())
				require.Equal(t, "https://opentelemetry.io/schemas/1.26.0", refs.At(0).SchemaUrl())
				require.Equal(t, invalidEntityType, refs.At(0).Type())
				requireBytesAttribute(t, resource.Attributes(), invalidEntityType, invalidUTF8())
			},
		},
		{
			name: "resource entity ref id keys",
			poison: func(traces ptrace.Traces) {
				ref := entity.ResourceEntityRefs(traces.ResourceSpans().AppendEmpty().Resource()).AppendEmpty()
				ref.SetType("service")
				ref.IdKeys().Append("service.name", invalidUTF8(), invalidUTF8()+"x")
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				resource := traces.ResourceSpans().At(0).Resource()
				refs := entity.ResourceEntityRefs(resource)
				require.Equal(t, 1, refs.Len())
				require.Equal(t, "service", refs.At(0).Type())
				require.Equal(t, []string{"service.name", invalidEntityIDKey, invalidEntityIDKey}, refs.At(0).IdKeys().AsRaw())
				requireBytesAttribute(t, resource.Attributes(), invalidEntityIDKey+"-1", invalidUTF8())
				requireBytesAttribute(t, resource.Attributes(), invalidEntityIDKey+"-2", invalidUTF8()+"x")
			},
		},
		{
			name: "resource entity ref description key",
			poison: func(traces ptrace.Traces) {
				ref := entity.ResourceEntityRefs(traces.ResourceSpans().AppendEmpty().Resource()).AppendEmpty()
				ref.SetType("service")
				ref.DescriptionKeys().Append("service.version", invalidUTF8())
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				resource := traces.ResourceSpans().At(0).Resource()
				refs := entity.ResourceEntityRefs(resource)
				require.Equal(t, 1, refs.Len())
				require.Equal(t, []string{"service.version", invalidEntityDescKey}, refs.At(0).DescriptionKeys().AsRaw())
				requireBytesAttribute(t, resource.Attributes(), invalidEntityDescKey+"-1", invalidUTF8())
			},
		},
	}

	for _, test := range tests {
		for _, readOnly := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/readonly=%t", test.name, readOnly), func(t *testing.T) {
				traces := ptrace.NewTraces()
				test.poison(traces)
				if readOnly {
					traces.MarkReadOnly()
				}
				sanitized := NewUTF8Sanitizer()(traces)
				if readOnly {
					require.False(t, sanitized.IsReadOnly())
				}
				test.check(t, sanitized)
			})
		}
	}
}

func TestUTF8Sanitizer_SanitizesNestedValues(t *testing.T) {
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
			expected: map[string]any{"key": []byte(invalidUTF8())},
		},
		{
			name: "string in slice",
			value: func(v pcommon.Value) {
				slice := v.SetEmptySlice()
				slice.AppendEmpty().SetStr("valid")
				slice.AppendEmpty().SetStr(invalidUTF8())
			},
			expected: []any{"valid", []byte(invalidUTF8())},
		},
		{
			name: "key in map",
			value: func(v pcommon.Value) {
				v.SetEmptyMap().PutStr(invalidUTF8(), "value")
			},
			expected: map[string]any{invalidTagKey + "-1": []byte(invalidUTF8() + ":value")},
		},
		{
			name: "string in map in slice",
			value: func(v pcommon.Value) {
				v.SetEmptySlice().AppendEmpty().SetEmptyMap().PutStr("key", invalidUTF8())
			},
			expected: []any{map[string]any{"key": []byte(invalidUTF8())}},
		},
		{
			name: "string in slice in slice",
			value: func(v pcommon.Value) {
				inner := v.SetEmptySlice().AppendEmpty().SetEmptySlice()
				inner.AppendEmpty().SetStr("valid")
				inner.AppendEmpty().SetStr(invalidUTF8())
			},
			expected: []any{[]any{"valid", []byte(invalidUTF8())}},
		},
		{
			name: "int in map",
			value: func(v pcommon.Value) {
				m := v.SetEmptyMap()
				m.PutInt("count", 7)
				m.PutStr("key", invalidUTF8())
			},
			expected: map[string]any{"count": int64(7), "key": []byte(invalidUTF8())},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			traces := ptrace.NewTraces()
			test.value(appendTestSpan(traces).Attributes().PutEmpty("nested"))
			sanitized := NewUTF8Sanitizer()(traces)
			value, ok := testSpan(sanitized).Attributes().Get("nested")
			require.True(t, ok)
			require.Equal(t, test.expected, value.AsRaw())
		})
	}
}

func TestUTF8Sanitizer_RecordsEveryInvalidKey(t *testing.T) {
	// pdata replaces an existing key on Put, so repeated keys are produced on
	// the wire: two keys of equal length are swapped for the same invalid bytes.
	traces := ptrace.NewTraces()
	attributes := appendTestSpan(traces).Attributes()
	attributes.PutStr("keep", "yes")
	attributes.PutStr("AAAA", "v1")
	attributes.PutStr("BBBB", "v2")
	buf, err := (&ptrace.ProtoMarshaler{}).MarshalTraces(traces)
	require.NoError(t, err)
	invalid := []byte(invalidUTF8())
	require.Len(t, invalid, len("AAAA"))
	buf = bytes.ReplaceAll(buf, []byte("AAAA"), invalid)
	buf = bytes.ReplaceAll(buf, []byte("BBBB"), invalid)
	decoded, err := (&ptrace.ProtoUnmarshaler{}).UnmarshalTraces(buf)
	require.NoError(t, err)

	sanitized := NewUTF8Sanitizer()(decoded)

	got := testSpan(sanitized).Attributes()
	require.Equal(t, 3, got.Len())
	kept, ok := got.Get("keep")
	require.True(t, ok)
	require.Equal(t, "yes", kept.Str())
	_, ok = got.Get(invalidUTF8())
	require.False(t, ok)
	requireBytesAttribute(t, got, invalidTagKey+"-1", invalidUTF8()+":v1")
	requireBytesAttribute(t, got, invalidTagKey+"-2", invalidUTF8()+":v2")
}

func TestUTF8Sanitizer_KeepsValidTraces(t *testing.T) {
	traces := tracesWithValidStrings()
	traces.MarkReadOnly()

	sanitized := NewUTF8Sanitizer()(traces)

	require.True(t, sanitized.IsReadOnly())
	span := testSpan(sanitized)
	require.Equal(t, "span", span.Name())
	require.Equal(t, "vendor=value", span.TraceState().AsRaw())
	require.Equal(t, "ok", span.Status().Message())
	event := span.Events().At(0)
	require.Equal(t, "event", event.Name())
	link := span.Links().At(0)
	require.Equal(t, "vendor=value", link.TraceState().AsRaw())
	scope := sanitized.ResourceSpans().At(0).ScopeSpans().At(0)
	require.Equal(t, "https://opentelemetry.io/schemas/1.26.0", scope.SchemaUrl())
	require.Equal(t, "scope", scope.Scope().Name())
	require.Equal(t, "v1", scope.Scope().Version())
	resourceSpans := sanitized.ResourceSpans().At(0)
	require.Equal(t, "https://opentelemetry.io/schemas/1.26.0", resourceSpans.SchemaUrl())
	refs := entity.ResourceEntityRefs(resourceSpans.Resource())
	require.Equal(t, 1, refs.Len())
	require.Equal(t, "service", refs.At(0).Type())
	require.Equal(t, []string{"service.name"}, refs.At(0).IdKeys().AsRaw())
	require.Equal(t, []string{"service.version"}, refs.At(0).DescriptionKeys().AsRaw())
	_, ok := span.Attributes().Get(invalidSpanName)
	require.False(t, ok)
}

func tracesWithValidStrings() ptrace.Traces {
	traces := ptrace.NewTraces()
	resourceSpans := traces.ResourceSpans().AppendEmpty()
	resourceSpans.SetSchemaUrl("https://opentelemetry.io/schemas/1.26.0")
	resource := resourceSpans.Resource()
	resource.Attributes().PutStr("service.name", "service")
	resource.Attributes().PutEmptyMap("map").PutStr("key", "value")
	resource.Attributes().PutEmptySlice("slice").AppendEmpty().SetStr("value")
	ref := entity.ResourceEntityRefs(resource).AppendEmpty()
	ref.SetSchemaUrl("https://opentelemetry.io/schemas/1.26.0")
	ref.SetType("service")
	ref.IdKeys().Append("service.name")
	ref.DescriptionKeys().Append("service.version")

	scopeSpans := resourceSpans.ScopeSpans().AppendEmpty()
	scopeSpans.SetSchemaUrl("https://opentelemetry.io/schemas/1.26.0")
	scope := scopeSpans.Scope()
	scope.SetName("scope")
	scope.SetVersion("v1")
	scope.Attributes().PutStr("key", "value")

	span := scopeSpans.Spans().AppendEmpty()
	span.SetName("span")
	span.TraceState().FromRaw("vendor=value")
	span.Status().SetMessage("ok")
	span.Attributes().PutStr("key", "value")
	event := span.Events().AppendEmpty()
	event.SetName("event")
	event.Attributes().PutStr("key", "value")
	link := span.Links().AppendEmpty()
	link.TraceState().FromRaw("vendor=value")
	link.Attributes().PutStr("key", "value")
	return traces
}

func appendTestSpan(traces ptrace.Traces) ptrace.Span {
	return traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
}

func testSpan(traces ptrace.Traces) ptrace.Span {
	return traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
}
