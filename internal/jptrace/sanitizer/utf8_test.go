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

func TestUTF8Sanitizer_RemovesInvalidKeys(t *testing.T) {
	k1 := invalidUTF8() + "-1"
	k2 := invalidUTF8() + "-2"

	traces := ptrace.NewTraces()
	attributes := traces.ResourceSpans().AppendEmpty().Resource().Attributes()
	attributes.PutStr(k1, "v1")
	attributes.PutStr(k2, "v2")

	sanitized := NewUTF8Sanitizer()(traces)
	got := sanitized.ResourceSpans().At(0).Resource().Attributes()
	_, ok := got.Get(k1)
	require.False(t, ok)
	_, ok = got.Get(k2)
	require.False(t, ok)
	v1, ok := got.Get(utf8Replacement + "-1")
	require.True(t, ok)
	require.Equal(t, "v1", v1.Str())
	v2, ok := got.Get(utf8Replacement + "-2")
	require.True(t, ok)
	require.Equal(t, "v2", v2.Str())
	require.Equal(t, 2, got.Len())
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

func TestUTF8Sanitizer_StrictProtobufDecode(t *testing.T) {
	for _, readOnly := range []bool{false, true} {
		t.Run(fmt.Sprintf("batch/readonly=%t", readOnly), func(t *testing.T) {
			traces := tracesWithInvalidUTF8()
			require.Error(t, unmarshalStrict(t, traces))
			if readOnly {
				traces.MarkReadOnly()
			}

			sanitized := NewUTF8Sanitizer()(traces)

			require.NoError(t, unmarshalStrict(t, sanitized))
			if readOnly {
				require.True(t, traces.IsReadOnly())
				require.False(t, sanitized.IsReadOnly())
				original, ok := traces.ResourceSpans().At(0).Resource().Attributes().Get("resource")
				require.True(t, ok)
				require.Equal(t, "res"+invalidUTF8(), original.Str())
			}
			assertRewritten(t, sanitized)
		})
	}
}

func TestUTF8Sanitizer_SingleInvalidField(t *testing.T) {
	tests := []struct {
		name     string
		poison   func(ptrace.Traces)
		original func(ptrace.Traces) string
		check    func(*testing.T, ptrace.Traces)
	}{
		{
			name: "resource schema url",
			poison: func(traces ptrace.Traces) {
				traces.ResourceSpans().At(0).SetSchemaUrl(invalidUTF8())
			},
			original: func(traces ptrace.Traces) string {
				return traces.ResourceSpans().At(0).SchemaUrl()
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				require.Equal(t, utf8Replacement, traces.ResourceSpans().At(0).SchemaUrl())
			},
		},
		{
			name: "scope schema url",
			poison: func(traces ptrace.Traces) {
				traces.ResourceSpans().At(0).ScopeSpans().At(0).SetSchemaUrl(invalidUTF8())
			},
			original: func(traces ptrace.Traces) string {
				return traces.ResourceSpans().At(0).ScopeSpans().At(0).SchemaUrl()
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				require.Equal(t, utf8Replacement, traces.ResourceSpans().At(0).ScopeSpans().At(0).SchemaUrl())
			},
		},
		{
			name: "scope name",
			poison: func(traces ptrace.Traces) {
				traces.ResourceSpans().At(0).ScopeSpans().At(0).Scope().SetName(invalidUTF8())
			},
			original: func(traces ptrace.Traces) string {
				return traces.ResourceSpans().At(0).ScopeSpans().At(0).Scope().Name()
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				require.Equal(t, utf8Replacement, traces.ResourceSpans().At(0).ScopeSpans().At(0).Scope().Name())
			},
		},
		{
			name: "scope version",
			poison: func(traces ptrace.Traces) {
				traces.ResourceSpans().At(0).ScopeSpans().At(0).Scope().SetVersion(invalidUTF8())
			},
			original: func(traces ptrace.Traces) string {
				return traces.ResourceSpans().At(0).ScopeSpans().At(0).Scope().Version()
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				require.Equal(t, utf8Replacement, traces.ResourceSpans().At(0).ScopeSpans().At(0).Scope().Version())
			},
		},
		{
			name: "scope attributes",
			poison: func(traces ptrace.Traces) {
				traces.ResourceSpans().At(0).ScopeSpans().At(0).Scope().Attributes().PutStr("scope", invalidUTF8())
			},
			original: func(traces ptrace.Traces) string {
				value, _ := traces.ResourceSpans().At(0).ScopeSpans().At(0).Scope().Attributes().Get("scope")
				return value.Str()
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				requireStringAttr(t, traces.ResourceSpans().At(0).ScopeSpans().At(0).Scope().Attributes(), "scope", utf8Replacement)
			},
		},
		{
			name: "span name",
			poison: func(traces ptrace.Traces) {
				testSpan(traces).SetName(invalidUTF8())
			},
			original: func(traces ptrace.Traces) string {
				return testSpan(traces).Name()
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				require.Equal(t, utf8Replacement, testSpan(traces).Name())
			},
		},
		{
			name: "status message",
			poison: func(traces ptrace.Traces) {
				testSpan(traces).Status().SetMessage(invalidUTF8())
			},
			original: func(traces ptrace.Traces) string {
				return testSpan(traces).Status().Message()
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				require.Equal(t, utf8Replacement, testSpan(traces).Status().Message())
			},
		},
		{
			name: "span tracestate",
			poison: func(traces ptrace.Traces) {
				testSpan(traces).TraceState().FromRaw(invalidUTF8())
			},
			original: func(traces ptrace.Traces) string {
				return testSpan(traces).TraceState().AsRaw()
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				require.Empty(t, testSpan(traces).TraceState().AsRaw())
			},
		},
		{
			name: "event name",
			poison: func(traces ptrace.Traces) {
				testSpan(traces).Events().At(0).SetName(invalidUTF8())
			},
			original: func(traces ptrace.Traces) string {
				return testSpan(traces).Events().At(0).Name()
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				require.Equal(t, utf8Replacement, testSpan(traces).Events().At(0).Name())
			},
		},
		{
			name: "event attribute",
			poison: func(traces ptrace.Traces) {
				testSpan(traces).Events().At(0).Attributes().PutStr("event", invalidUTF8())
			},
			original: func(traces ptrace.Traces) string {
				value, _ := testSpan(traces).Events().At(0).Attributes().Get("event")
				return value.Str()
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				requireStringAttr(t, testSpan(traces).Events().At(0).Attributes(), "event", utf8Replacement)
			},
		},
		{
			name: "link tracestate",
			poison: func(traces ptrace.Traces) {
				testSpan(traces).Links().At(0).TraceState().FromRaw(invalidUTF8())
			},
			original: func(traces ptrace.Traces) string {
				return testSpan(traces).Links().At(0).TraceState().AsRaw()
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				require.Empty(t, testSpan(traces).Links().At(0).TraceState().AsRaw())
			},
		},
		{
			name: "link attribute",
			poison: func(traces ptrace.Traces) {
				testSpan(traces).Links().At(0).Attributes().PutStr("link", invalidUTF8())
			},
			original: func(traces ptrace.Traces) string {
				value, _ := testSpan(traces).Links().At(0).Attributes().Get("link")
				return value.Str()
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				requireStringAttr(t, testSpan(traces).Links().At(0).Attributes(), "link", utf8Replacement)
			},
		},
		{
			name: "nested map value",
			poison: func(traces ptrace.Traces) {
				testSpan(traces).Attributes().PutEmptyMap("nested").PutStr("k", invalidUTF8())
			},
			original: func(traces ptrace.Traces) string {
				value, _ := testSpan(traces).Attributes().Get("nested")
				inner, _ := value.Map().Get("k")
				return inner.Str()
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				value, ok := testSpan(traces).Attributes().Get("nested")
				require.True(t, ok)
				inner, ok := value.Map().Get("k")
				require.True(t, ok)
				require.Equal(t, utf8Replacement, inner.Str())
			},
		},
		{
			name: "nested slice value",
			poison: func(traces ptrace.Traces) {
				testSpan(traces).Attributes().PutEmptySlice("items").AppendEmpty().SetStr(invalidUTF8())
			},
			original: func(traces ptrace.Traces) string {
				value, _ := testSpan(traces).Attributes().Get("items")
				return value.Slice().At(0).Str()
			},
			check: func(t *testing.T, traces ptrace.Traces) {
				value, ok := testSpan(traces).Attributes().Get("items")
				require.True(t, ok)
				require.Equal(t, utf8Replacement, value.Slice().At(0).Str())
			},
		},
	}
	for _, test := range tests {
		for _, readOnly := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/readonly=%t", test.name, readOnly), func(t *testing.T) {
				traces := tracesWithValidUTF8()
				test.poison(traces)
				require.Error(t, unmarshalStrict(t, traces))
				invalid := test.original(traces)
				if readOnly {
					traces.MarkReadOnly()
				}

				sanitized := NewUTF8Sanitizer()(traces)

				require.NoError(t, unmarshalStrict(t, sanitized))
				test.check(t, sanitized)
				if readOnly {
					require.Equal(t, invalid, test.original(traces))
					require.False(t, utf8.ValidString(test.original(traces)))
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

func TestUTF8Sanitizer_KeepsValidTraces(t *testing.T) {
	traces := tracesWithValidUTF8()
	traces.MarkReadOnly()

	sanitized := NewUTF8Sanitizer()(traces)

	require.True(t, sanitized.IsReadOnly())
	require.Equal(t, "span", testSpan(sanitized).Name())
	require.Equal(t, "vendor=value", testSpan(sanitized).TraceState().AsRaw())
}

func tracesWithInvalidUTF8() ptrace.Traces {
	traces := ptrace.NewTraces()
	resourceSpans := traces.ResourceSpans().AppendEmpty()
	resourceSpans.SetSchemaUrl("https://opentelemetry.io/schemas/1.26.0")
	resource := resourceSpans.Resource()
	resource.Attributes().PutStr("resource", "res"+invalidUTF8())
	resource.Attributes().PutStr("bad"+invalidUTF8(), "resource-key")

	scopeSpans := resourceSpans.ScopeSpans().AppendEmpty()
	scopeSpans.SetSchemaUrl(invalidUTF8())
	scope := scopeSpans.Scope()
	scope.SetName("scope" + invalidUTF8())
	scope.SetVersion("v1")
	scope.Attributes().PutStr("scope", invalidUTF8())

	span := scopeSpans.Spans().AppendEmpty()
	span.SetName("span" + invalidUTF8())
	span.TraceState().FromRaw("vendor=value" + invalidUTF8())
	span.Status().SetMessage("status" + invalidUTF8())
	span.Attributes().PutStr("keep", "yes")
	span.Attributes().PutStr("span", invalidUTF8())
	nested := span.Attributes().PutEmptyMap("map")
	nested.PutStr("inner", "nested"+invalidUTF8())
	nested.PutStr(invalidUTF8(), "nested-key")
	slice := nested.PutEmptySlice("slice")
	slice.AppendEmpty().SetStr("item" + invalidUTF8())
	slice.AppendEmpty().SetEmptyMap().PutStr("k", invalidUTF8())

	event := span.Events().AppendEmpty()
	event.SetName("event" + invalidUTF8())
	event.Attributes().PutStr("event", invalidUTF8())

	link := span.Links().AppendEmpty()
	link.TraceState().FromRaw(invalidUTF8())
	link.Attributes().PutStr("link", "link"+invalidUTF8())
	return traces
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

func assertRewritten(t *testing.T, traces ptrace.Traces) {
	t.Helper()
	resourceSpans := traces.ResourceSpans().At(0)
	require.Equal(t, "https://opentelemetry.io/schemas/1.26.0", resourceSpans.SchemaUrl())
	resourceAttrs := resourceSpans.Resource().Attributes()
	require.Equal(t, 2, resourceAttrs.Len())
	requireStringAttr(t, resourceAttrs, "resource", "res"+utf8Replacement)
	requireStringAttr(t, resourceAttrs, "bad"+utf8Replacement, "resource-key")
	_, ok := resourceAttrs.Get("bad" + invalidUTF8())
	require.False(t, ok)

	scopeSpans := resourceSpans.ScopeSpans().At(0)
	require.Equal(t, utf8Replacement, scopeSpans.SchemaUrl())
	scope := scopeSpans.Scope()
	require.Equal(t, "scope"+utf8Replacement, scope.Name())
	require.Equal(t, "v1", scope.Version())
	requireStringAttr(t, scope.Attributes(), "scope", utf8Replacement)

	span := scopeSpans.Spans().At(0)
	require.Equal(t, "span"+utf8Replacement, span.Name())
	require.Empty(t, span.TraceState().AsRaw())
	require.Equal(t, "status"+utf8Replacement, span.Status().Message())
	require.Equal(t, 3, span.Attributes().Len())
	requireStringAttr(t, span.Attributes(), "keep", "yes")
	requireStringAttr(t, span.Attributes(), "span", utf8Replacement)
	nested, ok := span.Attributes().Get("map")
	require.True(t, ok)
	require.Equal(t, map[string]any{
		"inner":         "nested" + utf8Replacement,
		utf8Replacement: "nested-key",
		"slice": []any{
			"item" + utf8Replacement,
			map[string]any{"k": utf8Replacement},
		},
	}, nested.AsRaw())

	event := span.Events().At(0)
	require.Equal(t, "event"+utf8Replacement, event.Name())
	requireStringAttr(t, event.Attributes(), "event", utf8Replacement)

	link := span.Links().At(0)
	require.Empty(t, link.TraceState().AsRaw())
	requireStringAttr(t, link.Attributes(), "link", "link"+utf8Replacement)
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

func testSpan(traces ptrace.Traces) ptrace.Span {
	return traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
}
