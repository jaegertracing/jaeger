// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/proto/api_v3"
)

func TestUIAdvertisesNestedSearchCapabilities(t *testing.T) {
	handler, err := newStaticAssetsHandler(
		&QueryOptions{UIConfig: UIConfig{AssetsPath: "fixture"}},
		withCaps(BackendCapabilities{ArchiveStorage: true, MetricsStorage: true}),
		zap.NewNop(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, handler.Close()) })
	html := string(handler.deriveIndexHTML(t.Context()))
	parts := strings.SplitN(html, "JAEGER_BACKEND_CAPABILITIES = ", 2)
	require.Len(t, parts, 2)
	var blob map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(strings.NewReader(parts[1])).Decode(&blob))
	require.Contains(t, blob, "search", "the UI bootstrap must advertise the full search capability object")
	require.JSONEq(t, `{}`, string(blob["search"]))
	require.Equal(t, "true", string(blob["archiveStorage"]))
	require.Equal(t, "true", string(blob["metricsStorage"]))
	require.Equal(t, "false", string(blob["searchWithoutServiceName"]))
	require.Equal(t, "false", string(blob["aiAssistant"]))
}

func TestUIReflectsLatestSearchCapabilities(t *testing.T) {
	search := &api_v3.SearchCapabilities{}
	handler, err := newStaticAssetsHandler(
		&QueryOptions{UIConfig: UIConfig{AssetsPath: "fixture"}},
		func(context.Context) BackendCapabilities {
			return BackendCapabilities{Search: search, SearchWithoutServiceName: search.WithoutServiceName}
		},
		zap.NewNop(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, handler.Close()) })
	for _, expected := range []string{`{}`, `{"withoutServiceName":true,"spanSearch":true}`} {
		html := string(handler.deriveIndexHTML(t.Context()))
		parts := strings.SplitN(html, "JAEGER_BACKEND_CAPABILITIES = ", 2)
		require.Len(t, parts, 2)
		var blob map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(strings.NewReader(parts[1])).Decode(&blob))
		require.JSONEq(t, expected, string(blob["search"]))
		var withoutServiceName bool
		require.NoError(t, json.Unmarshal(blob["searchWithoutServiceName"], &withoutServiceName))
		require.Equal(t, search.WithoutServiceName, withoutServiceName)
		search = &api_v3.SearchCapabilities{WithoutServiceName: true, SpanSearch: true}
	}
}

func TestUIEscapesSearchCapabilityMetadata(t *testing.T) {
	handler, err := newStaticAssetsHandler(
		&QueryOptions{UIConfig: UIConfig{AssetsPath: "fixture"}},
		withCaps(BackendCapabilities{Search: &api_v3.SearchCapabilities{
			Filter: &api_v3.FilterCapabilities{Operators: []string{"$1", "</script><script>alert(1)</script>"}},
		}}),
		zap.NewNop(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, handler.Close()) })
	html := string(handler.deriveIndexHTML(t.Context()))
	require.NotContains(t, html, "</script><script>alert(1)</script>")
	parts := strings.SplitN(html, "JAEGER_BACKEND_CAPABILITIES = ", 2)
	require.Len(t, parts, 2)
	var blob map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(strings.NewReader(parts[1])).Decode(&blob))
	require.JSONEq(t, `{"filter":{"operators":["$1","</script><script>alert(1)</script>"]}}`, string(blob["search"]))
}
