// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParamResolver_canonicalOnly(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/?startTime=2020-01-01T00:00:00Z", http.NoBody)
	resolver := newParamResolver(r)

	v, name, ok := resolver.Resolve(paramStartTime, paramStartTimeDeprecated)
	require.True(t, ok)
	assert.Equal(t, "2020-01-01T00:00:00Z", v)
	assert.Equal(t, paramStartTime, name)
	assert.Nil(t, resolver.DeprecatedParamsUsed())
}

func TestParamResolver_deprecatedOnly(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/?start_time=2020-01-01T00:00:00Z", http.NoBody)
	resolver := newParamResolver(r)

	v, name, ok := resolver.Resolve(paramStartTime, paramStartTimeDeprecated)
	require.True(t, ok)
	assert.Equal(t, "2020-01-01T00:00:00Z", v)
	assert.Equal(t, paramStartTimeDeprecated, name)
	assert.Equal(t, []string{paramStartTimeDeprecated}, resolver.DeprecatedParamsUsed())
}

func TestParamResolver_canonicalWinsOverDeprecated(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/?startTime=A&start_time=B", http.NoBody)
	resolver := newParamResolver(r)

	v, name, ok := resolver.Resolve(paramStartTime, paramStartTimeDeprecated)
	require.True(t, ok)
	assert.Equal(t, "A", v)
	assert.Equal(t, paramStartTime, name)
	assert.Nil(t, resolver.DeprecatedParamsUsed())
}

func TestParamResolver_emptyValueTreatedAsAbsent(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/?start_time=", http.NoBody)
	resolver := newParamResolver(r)

	_, _, ok := resolver.Resolve(paramStartTime, paramStartTimeDeprecated)
	assert.False(t, ok)
	assert.Nil(t, resolver.DeprecatedParamsUsed())
}

func TestParamResolver_firstValueWins(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/?startTime=A&startTime=B", http.NoBody)
	resolver := newParamResolver(r)

	v, _, ok := resolver.Resolve(paramStartTime, paramStartTimeDeprecated)
	require.True(t, ok)
	assert.Equal(t, "A", v)
}

func TestParamResolver_dottedParamNames(t *testing.T) {
	q := url.Values{}
	q.Set("query.serviceName", "bar")
	r := httptest.NewRequest(http.MethodGet, "/?"+q.Encode(), http.NoBody)
	resolver := newParamResolver(r)

	v, name, ok := resolver.Resolve(paramServiceName, paramServiceNameDeprecated)
	require.True(t, ok)
	assert.Equal(t, "bar", v)
	assert.Equal(t, paramServiceName, name)
}

func TestParamResolver_mixedVersionClient(t *testing.T) {
	q := url.Values{}
	q.Set(paramServiceNameDeprecated, "foo")
	q.Set(paramServiceName, "bar")
	r := httptest.NewRequest(http.MethodGet, "/?"+q.Encode(), http.NoBody)
	resolver := newParamResolver(r)

	v, name, ok := resolver.Resolve(paramServiceName, paramServiceNameDeprecated)
	require.True(t, ok)
	assert.Equal(t, "bar", v)
	assert.Equal(t, paramServiceName, name)
	assert.Nil(t, resolver.DeprecatedParamsUsed())
}

func TestParamResolver_searchDepthTripleAlias(t *testing.T) {
	t.Run("canonical wins over num_traces and search_depth", func(t *testing.T) {
		q := url.Values{}
		q.Set(paramSearchDepth, "5")
		q.Set(paramNumTracesDeprecated, "10")
		q.Set(paramSearchDepthDeprecated, "20")
		r := httptest.NewRequest(http.MethodGet, "/?"+q.Encode(), http.NoBody)
		resolver := newParamResolver(r)

		v, name, ok := resolver.Resolve(paramSearchDepth, paramNumTracesDeprecated, paramSearchDepthDeprecated)
		require.True(t, ok)
		assert.Equal(t, "5", v)
		assert.Equal(t, paramSearchDepth, name)
		assert.Nil(t, resolver.DeprecatedParamsUsed())
	})
	t.Run("num_traces deprecated when alone", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/?"+paramNumTracesDeprecated+"=7", http.NoBody)
		resolver := newParamResolver(r)

		v, name, ok := resolver.Resolve(paramSearchDepth, paramNumTracesDeprecated, paramSearchDepthDeprecated)
		require.True(t, ok)
		assert.Equal(t, "7", v)
		assert.Equal(t, paramNumTracesDeprecated, name)
		assert.Equal(t, []string{paramNumTracesDeprecated}, resolver.DeprecatedParamsUsed())
	})
}

func TestParamResolver_multipleDeprecatedParams(t *testing.T) {
	q := url.Values{}
	q.Set(paramStartTimeDeprecated, "2020-01-01T00:00:00Z")
	q.Set(paramSpanKindDeprecated, "server")
	r := httptest.NewRequest(http.MethodGet, "/?"+q.Encode(), http.NoBody)
	resolver := newParamResolver(r)

	_, _, ok := resolver.Resolve(paramStartTime, paramStartTimeDeprecated)
	require.True(t, ok)
	_, _, ok = resolver.Resolve(paramSpanKind, paramSpanKindDeprecated)
	require.True(t, ok)
	assert.ElementsMatch(t, []string{paramStartTimeDeprecated, paramSpanKindDeprecated}, resolver.DeprecatedParamsUsed())
}

func BenchmarkParamResolver_NoDeprecated(b *testing.B) {
	r := httptest.NewRequest(http.MethodGet, "/?startTime=2020-01-01T00:00:00Z&endTime=2021-01-01T00:00:00Z", http.NoBody)
	b.ReportAllocs()
	for b.Loop() {
		resolver := newParamResolver(r)
		resolver.Resolve(paramStartTime, paramStartTimeDeprecated)
		resolver.Resolve(paramEndTime, paramEndTimeDeprecated)
	}
}

func BenchmarkParamResolver_WithDeprecated(b *testing.B) {
	r := httptest.NewRequest(http.MethodGet, "/?start_time=2020-01-01T00:00:00Z", http.NoBody)
	b.ReportAllocs()
	for b.Loop() {
		resolver := newParamResolver(r)
		resolver.Resolve(paramStartTime, paramStartTimeDeprecated)
		_ = resolver.DeprecatedParamsUsed()
	}
}
