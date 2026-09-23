// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package anonymizer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// newTrace builds a trace carrying one of every piece of text the anonymizer handles.
func newTrace() ptrace.Traces {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "api")
	rs.Resource().Attributes().PutStr("host.name", "prod-db-7")

	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("com.example.client")
	ss.Scope().SetVersion("1.2.3")
	ss.Scope().Attributes().PutStr("team", "payments")

	span := ss.Spans().AppendEmpty()
	span.SetName("delete")
	span.SetKind(ptrace.SpanKindServer)
	span.TraceState().FromRaw("vendor=secret")
	span.Status().SetCode(ptrace.StatusCodeError)
	span.Status().SetMessage("user 42 not found")
	span.Attributes().PutStr("http.method", "DELETE")
	span.Attributes().PutStr("user.email", "a@example.com")
	span.Attributes().PutInt("http.status_code", 404)
	span.Attributes().PutStr("error", "boom")

	event := span.Events().AppendEmpty()
	event.SetName("exception")
	event.Attributes().PutStr("exception.message", "row 42 locked")

	link := span.Links().AppendEmpty()
	link.SetSpanID([8]byte{1})
	link.TraceState().FromRaw("vendor=secret")
	link.Attributes().PutStr("reason", "retry of order 42")
	return traces
}

func firstSpan(traces ptrace.Traces) (pcommon.Resource, ptrace.ScopeSpans, ptrace.Span) {
	rs := traces.ResourceSpans().At(0)
	ss := rs.ScopeSpans().At(0)
	return rs.Resource(), ss, ss.Spans().At(0)
}

func newAnonymizer(t *testing.T, options Options) *Anonymizer {
	a, err := New(filepath.Join(t.TempDir(), "mapping.json"), options, zap.NewNop())
	require.NoError(t, err)
	return a
}

func TestNew(t *testing.T) {
	t.Run("loads the mapping of an earlier run", func(t *testing.T) {
		mappingFile := filepath.Join(t.TempDir(), "mapping.json")
		require.NoError(t, os.WriteFile(mappingFile, []byte(`{
			"services": {"api": "hashed_api"},
			"operations": {"[api]:delete": "hashed_api_delete"}
		}`), PermUserRW))

		a, err := New(mappingFile, Options{}, zap.NewNop())
		require.NoError(t, err)
		assert.Equal(t, "hashed_api", a.mapServiceName("api"))
		assert.Equal(t, "hashed_api_delete", a.mapOperationName("api", "delete"))
	})

	t.Run("starts empty without an earlier mapping", func(t *testing.T) {
		a := newAnonymizer(t, Options{})
		assert.Empty(t, a.mapping.Services)
	})

	t.Run("refuses a mapping it cannot parse", func(t *testing.T) {
		mappingFile := filepath.Join(t.TempDir(), "mapping.json")
		require.NoError(t, os.WriteFile(mappingFile, []byte("not json"), PermUserRW))
		_, err := New(mappingFile, Options{}, zap.NewNop())
		require.ErrorContains(t, err, "cannot unmarshal previous mapping")
	})

	t.Run("refuses a mapping it cannot read", func(t *testing.T) {
		_, err := New(t.TempDir(), Options{}, zap.NewNop())
		require.ErrorContains(t, err, "cannot load previous mapping")
	})
}

func TestSaveMapping(t *testing.T) {
	a := newAnonymizer(t, Options{})
	a.mapServiceName("api")
	require.NoError(t, a.SaveMapping())

	reloaded, err := New(a.mappingFile, Options{}, zap.NewNop())
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"api": hash("api")}, reloaded.mapping.Services)

	a.mappingFile = t.TempDir() // a directory cannot be written as a file
	require.ErrorContains(t, a.SaveMapping(), "failed to write mapping file")
}

func TestHash(t *testing.T) {
	assert.Equal(t, "340d8765a4dda9c2", hash("foobar"))
}

func TestMapString(t *testing.T) {
	a := &Anonymizer{}
	assert.Equal(t, "hashed_foobar", a.mapString("foobar", map[string]string{"foobar": "hashed_foobar"}))
	m := map[string]string{}
	assert.Equal(t, "340d8765a4dda9c2", a.mapString("foobar", m))
	assert.Equal(t, map[string]string{"foobar": "340d8765a4dda9c2"}, m)
}

func TestAnonymizeTracesAllFalse(t *testing.T) {
	a := newAnonymizer(t, Options{})
	traces := newTrace()
	a.AnonymizeTraces(traces)
	resource, ss, span := firstSpan(traces)

	assert.Equal(t, map[string]any{"service.name": hash("api")}, resource.Attributes().AsRaw())
	assert.Equal(t, hash("[api]:delete"), span.Name())
	assert.Equal(t, map[string]any{
		"http.method":      "DELETE",
		"http.status_code": int64(404),
		"error":            true, // free text under the error key is replaced
	}, span.Attributes().AsRaw())
	assert.Equal(t, 0, span.Events().Len())

	assert.Empty(t, ss.Scope().Name())
	assert.Empty(t, ss.Scope().Version())
	assert.Equal(t, 0, ss.Scope().Attributes().Len())
	assert.Empty(t, span.Status().Message())
	assert.Empty(t, span.TraceState().AsRaw())
	assert.Equal(t, 0, span.Links().At(0).Attributes().Len())
	assert.Empty(t, span.Links().At(0).TraceState().AsRaw())

	// Structure is kept.
	assert.Equal(t, ptrace.SpanKindServer, span.Kind())
	assert.Equal(t, ptrace.StatusCodeError, span.Status().Code())
	assert.Equal(t, pcommon.SpanID([8]byte{1}), span.Links().At(0).SpanID())
}

func TestAnonymizeTracesAllTrue(t *testing.T) {
	a := newAnonymizer(t, Options{
		HashStandardTags: true,
		HashCustomTags:   true,
		HashLogs:         true,
		HashProcess:      true,
	})
	traces := newTrace()
	a.AnonymizeTraces(traces)
	resource, ss, span := firstSpan(traces)

	assert.Equal(t, map[string]any{
		hash("host.name"): hash("prod-db-7"),
		"service.name":    hash("api"),
	}, resource.Attributes().AsRaw())
	assert.Equal(t, map[string]any{
		hash("http.method"):      hash("DELETE"),
		hash("http.status_code"): hash("404"),
		hash("error"):            hash("true"),
		hash("user.email"):       hash("a@example.com"),
	}, span.Attributes().AsRaw())

	event := span.Events().At(0)
	assert.Equal(t, hash("exception"), event.Name())
	assert.Equal(t, map[string]any{hash("exception.message"): hash("row 42 locked")}, event.Attributes().AsRaw())

	assert.Equal(t, hash("com.example.client"), ss.Scope().Name())
	assert.Equal(t, hash("1.2.3"), ss.Scope().Version())
	assert.Equal(t, map[string]any{hash("team"): hash("payments")}, ss.Scope().Attributes().AsRaw())
	assert.Equal(t, hash("user 42 not found"), span.Status().Message())
	assert.Equal(t, map[string]any{hash("reason"): hash("retry of order 42")}, span.Links().At(0).Attributes().AsRaw())
	assert.Empty(t, span.TraceState().AsRaw())
}

func TestAnonymizeTracesKeepsStandardAttributesFirst(t *testing.T) {
	a := newAnonymizer(t, Options{HashCustomTags: true})
	traces := ptrace.NewTraces()
	span := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.Attributes().PutStr("custom", "x")
	span.Attributes().PutStr("http.method", "GET")

	a.AnonymizeTraces(traces)

	var keys []string
	for key := range span.Attributes().All() {
		keys = append(keys, key)
	}
	assert.Equal(t, []string{"http.method", hash("custom")}, keys)
}

func TestAnonymizeTracesWithoutServiceName(t *testing.T) {
	a := newAnonymizer(t, Options{HashProcess: true})
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("host.name", "h")
	rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty().SetName("op")

	a.AnonymizeTraces(traces)

	assert.Equal(t, map[string]any{hash("host.name"): hash("h")}, rs.Resource().Attributes().AsRaw())
	assert.Equal(t, hash("[]:op"), rs.ScopeSpans().At(0).Spans().At(0).Name())
}

func TestNormalizeError(t *testing.T) {
	tests := []struct {
		name  string
		value func(pcommon.Map)
		want  any
	}{
		{name: "boolean kept", value: func(m pcommon.Map) { m.PutBool("error", false) }, want: false},
		{name: "true as text kept", value: func(m pcommon.Map) { m.PutStr("error", "true") }, want: "true"},
		{name: "false as text kept", value: func(m pcommon.Map) { m.PutStr("error", "false") }, want: "false"},
		{name: "free text replaced", value: func(m pcommon.Map) { m.PutStr("error", "boom") }, want: true},
		{name: "number replaced", value: func(m pcommon.Map) { m.PutInt("error", 1) }, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newAnonymizer(t, Options{})
			traces := ptrace.NewTraces()
			span := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
			tt.value(span.Attributes())

			a.AnonymizeTraces(traces)

			got, ok := span.Attributes().Get("error")
			require.True(t, ok)
			assert.Equal(t, tt.want, got.AsRaw())
		})
	}
}
