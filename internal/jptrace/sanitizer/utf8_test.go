// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"encoding/hex"
	"fmt"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

func invalidUTF8() string {
	s, _ := hex.DecodeString("fefeffff")
	return string(s)
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
		expectedKey:   utf8Replacement,
		expectedValue: pcommon.NewValueStr("value"),
	},
	{
		name:          "valid key + invalid value",
		key:           "key",
		value:         invalidUTF8(),
		expectedKey:   "key",
		expectedValue: pcommon.NewValueStr(utf8Replacement),
	},
	{
		name:          "invalid key + invalid value",
		key:           invalidUTF8(),
		value:         invalidUTF8(),
		expectedKey:   utf8Replacement,
		expectedValue: pcommon.NewValueStr(utf8Replacement),
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
			sanitized := NewUTF8Sanitizer()(traces)
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
			sanitized := NewUTF8Sanitizer()(traces)
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
			sanitized := NewUTF8Sanitizer()(traces)
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
	span := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetName("span" + invalidUTF8())
	sanitized := NewUTF8Sanitizer()(traces)
	got := sanitized.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	require.Equal(t, "span"+utf8Replacement, got.Name())
	require.Equal(t, 0, got.Attributes().Len())
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
	sanitized := NewUTF8Sanitizer()(traces)
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

func TestUTF8Sanitizer_DoesNotSanitizeNonStringAttributeValue(t *testing.T) {
	traces := ptrace.NewTraces()
	traces.
		ResourceSpans().
		AppendEmpty().
		Resource().
		Attributes().
		PutInt("key", 99)
	sanitized := NewUTF8Sanitizer()(traces)
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
	sanitized := NewUTF8Sanitizer()(traces)
	attributes := sanitized.ResourceSpans().At(0).Resource().Attributes()
	_, ok := attributes.Get(invalidUTF8())
	require.False(t, ok)
	value, ok := attributes.Get(utf8Replacement)
	require.True(t, ok)
	require.EqualValues(t, 99, value.Int())
}

func TestUTF8Sanitizer_SanitizesMultipleAttributesWithInvalidKeys(t *testing.T) {
	k1 := invalidUTF8() + "-1"
	k2 := invalidUTF8() + "-2"
	traces := ptrace.NewTraces()
	attributes := traces.ResourceSpans().AppendEmpty().Resource().Attributes()
	attributes.PutStr(k1, "v1")
	attributes.PutStr(k2, "v2")

	sanitized := NewUTF8Sanitizer()(traces)
	got := sanitized.ResourceSpans().At(0).Resource().Attributes()
	require.Equal(t, 2, got.Len())
	v1, ok := got.Get(utf8Replacement + "-1")
	require.True(t, ok)
	require.Equal(t, "v1", v1.Str())
	v2, ok := got.Get(utf8Replacement + "-2")
	require.True(t, ok)
	require.Equal(t, "v2", v2.Str())
}

func TestUTF8Sanitizer_SingleInvalidField(t *testing.T) {
	tests := []struct {
		name   string
		poison func(ptrace.Traces)
		field  func(ptrace.Traces) string
		want   string
	}{
		{
			name:   "resource schema url",
			poison: func(traces ptrace.Traces) { traces.ResourceSpans().At(0).SetSchemaUrl(invalidUTF8()) },
			field:  func(traces ptrace.Traces) string { return traces.ResourceSpans().At(0).SchemaUrl() },
			want:   utf8Replacement,
		},
		{
			name: "scope schema url",
			poison: func(traces ptrace.Traces) {
				traces.ResourceSpans().At(0).ScopeSpans().At(0).SetSchemaUrl(invalidUTF8())
			},
			field: func(traces ptrace.Traces) string { return traces.ResourceSpans().At(0).ScopeSpans().At(0).SchemaUrl() },
			want:  utf8Replacement,
		},
		{
			name:   "scope name",
			poison: func(traces ptrace.Traces) { scopeOf(traces).SetName(invalidUTF8()) },
			field:  func(traces ptrace.Traces) string { return scopeOf(traces).Name() },
			want:   utf8Replacement,
		},
		{
			name:   "scope version",
			poison: func(traces ptrace.Traces) { scopeOf(traces).SetVersion(invalidUTF8()) },
			field:  func(traces ptrace.Traces) string { return scopeOf(traces).Version() },
			want:   utf8Replacement,
		},
		{
			name:   "scope attributes",
			poison: func(traces ptrace.Traces) { scopeOf(traces).Attributes().PutStr("scope", invalidUTF8()) },
			field:  func(traces ptrace.Traces) string { return attr(scopeOf(traces).Attributes(), "scope") },
			want:   utf8Replacement,
		},
		{
			name:   "span name",
			poison: func(traces ptrace.Traces) { testSpan(traces).SetName(invalidUTF8()) },
			field:  func(traces ptrace.Traces) string { return testSpan(traces).Name() },
			want:   utf8Replacement,
		},
		{
			name:   "status message",
			poison: func(traces ptrace.Traces) { testSpan(traces).Status().SetMessage(invalidUTF8()) },
			field:  func(traces ptrace.Traces) string { return testSpan(traces).Status().Message() },
			want:   utf8Replacement,
		},
		{
			name:   "span tracestate",
			poison: func(traces ptrace.Traces) { testSpan(traces).TraceState().FromRaw(invalidUTF8()) },
			field:  func(traces ptrace.Traces) string { return testSpan(traces).TraceState().AsRaw() },
		},
		{
			name:   "event name",
			poison: func(traces ptrace.Traces) { testSpan(traces).Events().At(0).SetName(invalidUTF8()) },
			field:  func(traces ptrace.Traces) string { return testSpan(traces).Events().At(0).Name() },
			want:   utf8Replacement,
		},
		{
			name: "event attribute",
			poison: func(traces ptrace.Traces) {
				testSpan(traces).Events().At(0).Attributes().PutStr("event", invalidUTF8())
			},
			field: func(traces ptrace.Traces) string { return attr(testSpan(traces).Events().At(0).Attributes(), "event") },
			want:  utf8Replacement,
		},
		{
			name:   "link tracestate",
			poison: func(traces ptrace.Traces) { testSpan(traces).Links().At(0).TraceState().FromRaw(invalidUTF8()) },
			field:  func(traces ptrace.Traces) string { return testSpan(traces).Links().At(0).TraceState().AsRaw() },
		},
		{
			name:   "link attribute",
			poison: func(traces ptrace.Traces) { testSpan(traces).Links().At(0).Attributes().PutStr("link", invalidUTF8()) },
			field:  func(traces ptrace.Traces) string { return attr(testSpan(traces).Links().At(0).Attributes(), "link") },
			want:   utf8Replacement,
		},
		{
			name: "nested map value",
			poison: func(traces ptrace.Traces) {
				testSpan(traces).Attributes().PutEmptyMap("nested").PutStr("k", invalidUTF8())
			},
			field: func(traces ptrace.Traces) string {
				value, _ := testSpan(traces).Attributes().Get("nested")
				return attr(value.Map(), "k")
			},
			want: utf8Replacement,
		},
		{
			name: "nested slice value",
			poison: func(traces ptrace.Traces) {
				testSpan(traces).Attributes().PutEmptySlice("items").AppendEmpty().SetStr(invalidUTF8())
			},
			field: func(traces ptrace.Traces) string {
				value, _ := testSpan(traces).Attributes().Get("items")
				return value.Slice().At(0).Str()
			},
			want: utf8Replacement,
		},
	}
	for _, test := range tests {
		for _, readOnly := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/readonly=%t", test.name, readOnly), func(t *testing.T) {
				traces := tracesWithValidUTF8()
				test.poison(traces)
				require.Error(t, unmarshalStrict(t, traces))
				invalid := test.field(traces)
				if readOnly {
					traces.MarkReadOnly()
				}

				sanitized := NewUTF8Sanitizer()(traces)

				require.NoError(t, unmarshalStrict(t, sanitized))
				require.Equal(t, test.want, test.field(sanitized))
				if readOnly {
					require.Equal(t, invalid, test.field(traces))
					require.False(t, utf8.ValidString(test.field(traces)))
				}
			})
		}
	}
}

func TestUTF8Sanitizer_DropsRepairedKeyWhenValidKeyExists(t *testing.T) {
	traces := ptrace.NewTraces()
	attributes := traces.ResourceSpans().AppendEmpty().Resource().Attributes()
	// The invalid key is inserted first. The valid U+FFFD key must still win.
	attributes.PutStr(invalidUTF8(), "from-invalid")
	attributes.PutStr(utf8Replacement, "from-valid")

	sanitized := NewUTF8Sanitizer()(traces)

	got := sanitized.ResourceSpans().At(0).Resource().Attributes()
	require.Equal(t, 1, got.Len())
	requireStringAttr(t, got, utf8Replacement, "from-valid")
	_, ok := got.Get(invalidUTF8())
	require.False(t, ok)
	require.NoError(t, unmarshalStrict(t, sanitized))
}

func TestUTF8Sanitizer_KeepsFirstRepairedKey(t *testing.T) {
	traces := ptrace.NewTraces()
	attributes := traces.ResourceSpans().AppendEmpty().Resource().Attributes()
	// Both bytes are invalid and repair to U+FFFD. Insertion order is the
	// pdata map iteration order.
	attributes.PutStr(string([]byte{0xff}), "first")
	attributes.PutStr(string([]byte{0xfe}), "second")

	sanitized := NewUTF8Sanitizer()(traces)

	got := sanitized.ResourceSpans().At(0).Resource().Attributes()
	require.Equal(t, 1, got.Len())
	requireStringAttr(t, got, utf8Replacement, "first")
	require.NoError(t, unmarshalStrict(t, sanitized))
}

func TestUTF8Sanitizer_ReadOnlyNestedInvalidKeys(t *testing.T) {
	traces := ptrace.NewTraces()
	attributes := traces.ResourceSpans().AppendEmpty().Resource().Attributes()
	attributes.PutEmptyMap("wrap").PutStr(invalidUTF8(), "in-map")
	attributes.PutEmptySlice("list").AppendEmpty().SetEmptyMap().PutStr(string([]byte{0xfe}), "in-slice")
	traces.MarkReadOnly()
	require.Error(t, unmarshalStrict(t, traces))

	sanitized := NewUTF8Sanitizer()(traces)

	require.False(t, sanitized.IsReadOnly())
	require.True(t, traces.IsReadOnly())
	originalMap, ok := traces.ResourceSpans().At(0).Resource().Attributes().Get("wrap")
	require.True(t, ok)
	_, ok = originalMap.Map().Get(invalidUTF8())
	require.True(t, ok)
	originalSlice, ok := traces.ResourceSpans().At(0).Resource().Attributes().Get("list")
	require.True(t, ok)
	_, ok = originalSlice.Slice().At(0).Map().Get(string([]byte{0xfe}))
	require.True(t, ok)

	require.NoError(t, unmarshalStrict(t, sanitized))
	got := sanitized.ResourceSpans().At(0).Resource().Attributes()
	repairedMap, ok := got.Get("wrap")
	require.True(t, ok)
	requireStringAttr(t, repairedMap.Map(), utf8Replacement, "in-map")
	repairedSlice, ok := got.Get("list")
	require.True(t, ok)
	requireStringAttr(t, repairedSlice.Slice().At(0).Map(), utf8Replacement, "in-slice")
}

func TestUTF8Sanitizer_PreservesValuesUnderInvalidKeys(t *testing.T) {
	traces := ptrace.NewTraces()
	attributes := traces.ResourceSpans().AppendEmpty().Resource().Attributes()
	attributes.PutEmptyMap(invalidUTF8()+"m").PutStr("k", "v")
	attributes.PutEmptySlice(invalidUTF8() + "s").AppendEmpty().SetInt(7)
	raw := []byte{0xff, 0xfe, 0xfd}
	attributes.PutEmptyBytes(invalidUTF8() + "b").FromRaw(raw)

	sanitized := NewUTF8Sanitizer()(traces)

	got := sanitized.ResourceSpans().At(0).Resource().Attributes()
	repairedMap, ok := got.Get(utf8Replacement + "m")
	require.True(t, ok)
	require.Equal(t, map[string]any{"k": "v"}, repairedMap.AsRaw())
	repairedSlice, ok := got.Get(utf8Replacement + "s")
	require.True(t, ok)
	require.Equal(t, []any{int64(7)}, repairedSlice.AsRaw())
	repairedBytes, ok := got.Get(utf8Replacement + "b")
	require.True(t, ok)
	require.Equal(t, raw, repairedBytes.Bytes().AsRaw())
	require.NoError(t, unmarshalStrict(t, sanitized))
}

func TestUTF8Sanitizer_ReadOnlyInvalidOnlyInLastSpan(t *testing.T) {
	traces := ptrace.NewTraces()
	for range 2 {
		resourceSpans := traces.ResourceSpans().AppendEmpty()
		resourceSpans.Resource().Attributes().PutStr("service.name", "svc")
		for range 2 {
			resourceSpans.ScopeSpans().AppendEmpty().Spans().AppendEmpty().SetName("span")
		}
	}
	traces.ResourceSpans().At(1).ScopeSpans().At(1).Spans().At(0).SetName(invalidUTF8())
	traces.MarkReadOnly()

	sanitized := NewUTF8Sanitizer()(traces)

	require.False(t, sanitized.IsReadOnly())
	require.Equal(t, invalidUTF8(), traces.ResourceSpans().At(1).ScopeSpans().At(1).Spans().At(0).Name())
	require.Equal(t, "span", sanitized.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Name())
	require.Equal(t, utf8Replacement, sanitized.ResourceSpans().At(1).ScopeSpans().At(1).Spans().At(0).Name())
	require.NoError(t, unmarshalStrict(t, sanitized))
}

func TestUTF8Sanitizer_InvalidKeysOnAttributeOwners(t *testing.T) {
	traces := tracesWithValidUTF8()
	key := invalidUTF8()
	traces.ResourceSpans().At(0).Resource().Attributes().PutStr(key, "resource")
	traces.ResourceSpans().At(0).ScopeSpans().At(0).Scope().Attributes().PutStr(key, "scope")
	testSpan(traces).Events().At(0).Attributes().PutStr(key, "event")
	testSpan(traces).Links().At(0).Attributes().PutStr(key, "link")
	traces.MarkReadOnly()

	sanitized := NewUTF8Sanitizer()(traces)

	require.False(t, sanitized.IsReadOnly())
	_, ok := traces.ResourceSpans().At(0).Resource().Attributes().Get(key)
	require.True(t, ok)
	require.NoError(t, unmarshalStrict(t, sanitized))
	requireStringAttr(t, sanitized.ResourceSpans().At(0).Resource().Attributes(), utf8Replacement, "resource")
	requireStringAttr(t, sanitized.ResourceSpans().At(0).ScopeSpans().At(0).Scope().Attributes(), utf8Replacement, "scope")
	requireStringAttr(t, testSpan(sanitized).Events().At(0).Attributes(), utf8Replacement, "event")
	requireStringAttr(t, testSpan(sanitized).Links().At(0).Attributes(), utf8Replacement, "link")
}

func TestUTF8Sanitizer_NestedMapKeyCollision(t *testing.T) {
	traces := ptrace.NewTraces()
	nested := traces.ResourceSpans().AppendEmpty().Resource().Attributes().PutEmptyMap("nested")
	nested.PutStr(string([]byte{0xff}), "first")
	nested.PutStr(utf8Replacement, "valid")
	nested.PutStr(string([]byte{0xfe}), "second")
	order := traces.ResourceSpans().At(0).Resource().Attributes().PutEmptyMap("order")
	order.PutStr(string([]byte{0xff}), "first")
	order.PutStr(string([]byte{0xfe}), "second")

	sanitized := NewUTF8Sanitizer()(traces)

	attributes := sanitized.ResourceSpans().At(0).Resource().Attributes()
	got, ok := attributes.Get("nested")
	require.True(t, ok)
	require.Equal(t, 1, got.Map().Len())
	requireStringAttr(t, got.Map(), utf8Replacement, "valid")
	ordered, ok := attributes.Get("order")
	require.True(t, ok)
	require.Equal(t, 1, ordered.Map().Len())
	requireStringAttr(t, ordered.Map(), utf8Replacement, "first")
	require.NoError(t, unmarshalStrict(t, sanitized))
}

func TestUTF8Sanitizer_KeepsValidTraces(t *testing.T) {
	traces := tracesWithValidUTF8()
	traces.MarkReadOnly()

	sanitized := NewUTF8Sanitizer()(traces)

	require.True(t, sanitized.IsReadOnly())
	require.Equal(t, "span", testSpan(sanitized).Name())
	require.Equal(t, "vendor=value", testSpan(sanitized).TraceState().AsRaw())
}

func tracesWithValidUTF8() ptrace.Traces {
	traces := ptrace.NewTraces()
	resourceSpans := traces.ResourceSpans().AppendEmpty()
	resourceSpans.SetSchemaUrl("https://opentelemetry.io/schemas/1.26.0")
	resource := resourceSpans.Resource()
	resource.Attributes().PutStr("service.name", "service")
	resource.Attributes().PutEmptyMap("map").PutStr("key", "value")
	resource.Attributes().PutEmptySlice("slice").AppendEmpty().SetStr("value")

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

func requireStringAttr(t *testing.T, attributes pcommon.Map, key, value string) {
	t.Helper()
	got, ok := attributes.Get(key)
	require.True(t, ok)
	require.Equal(t, value, got.Str())
}

func unmarshalStrict(t *testing.T, traces ptrace.Traces) error {
	t.Helper()
	buf, err := (&ptrace.ProtoMarshaler{}).MarshalTraces(traces)
	require.NoError(t, err)
	return proto.Unmarshal(buf, &collectortracev1.ExportTraceServiceRequest{})
}

func scopeOf(traces ptrace.Traces) pcommon.InstrumentationScope {
	return traces.ResourceSpans().At(0).ScopeSpans().At(0).Scope()
}

func attr(attributes pcommon.Map, key string) string {
	value, _ := attributes.Get(key)
	return value.Str()
}

func testSpan(traces ptrace.Traces) ptrace.Span {
	return traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
}
