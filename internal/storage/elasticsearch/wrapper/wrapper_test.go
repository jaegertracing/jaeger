// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package eswrapper

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	esv8 "github.com/elastic/go-elasticsearch/v9"
	"github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
)

const composableTemplateBody = `{"template":{"settings":{}}}`

func TestClientWrapper_CreateComposableTemplates(t *testing.T) {
	tests := []struct {
		name     string
		version  es.BackendVersion
		create   func(c ClientWrapper) error
		wantPath string
	}{
		{
			name:    "component template / v7 (olivere)",
			version: es.ElasticV7,
			create: func(c ClientWrapper) error {
				return c.CreateComponentTemplate(context.Background(), "jaeger-span-mappings", composableTemplateBody)
			},
			wantPath: "/_component_template/jaeger-span-mappings",
		},
		{
			name:    "component template / v8",
			version: es.ElasticV8,
			create: func(c ClientWrapper) error {
				return c.CreateComponentTemplate(context.Background(), "jaeger-span-mappings", composableTemplateBody)
			},
			wantPath: "/_component_template/jaeger-span-mappings",
		},
		{
			name:    "index template / v7 (olivere)",
			version: es.ElasticV7,
			create: func(c ClientWrapper) error {
				return c.CreateComposableIndexTemplate(context.Background(), "jaeger-span", composableTemplateBody)
			},
			wantPath: "/_index_template/jaeger-span",
		},
		{
			name:    "index template / v8",
			version: es.ElasticV8,
			create: func(c ClientWrapper) error {
				return c.CreateComposableIndexTemplate(context.Background(), "jaeger-span", composableTemplateBody)
			},
			wantPath: "/_index_template/jaeger-span",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotMethod, gotPath, gotBody string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath = r.Method, r.URL.Path
				b, _ := io.ReadAll(r.Body)
				gotBody = string(b)
				w.Header().Set("X-Elastic-Product", "Elasticsearch")
				w.Write([]byte(`{"acknowledged":true}`))
			}))
			defer srv.Close()

			require.NoError(t, tt.create(newTestWrapper(t, tt.version, srv.URL)))
			assert.Equal(t, http.MethodPut, gotMethod)
			assert.Equal(t, tt.wantPath, gotPath)
			assert.JSONEq(t, composableTemplateBody, gotBody)
		})
	}
}

func TestClientWrapper_CreateComposableTemplates_Error(t *testing.T) {
	for _, version := range []es.BackendVersion{es.ElasticV7, es.ElasticV8} {
		t.Run(version.String(), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-Elastic-Product", "Elasticsearch")
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":"bad request"}`))
			}))
			defer srv.Close()

			c := newTestWrapper(t, version, srv.URL)
			require.Error(t, c.CreateComponentTemplate(context.Background(), "jaeger-span-mappings", "{}"))
			require.Error(t, c.CreateComposableIndexTemplate(context.Background(), "jaeger-span", "{}"))
		})
	}
}

func newTestWrapper(t *testing.T, version es.BackendVersion, url string) ClientWrapper {
	t.Helper()
	if version.UsesV8API() {
		v8, err := esv8.NewClient(esv8.Config{Addresses: []string{url}})
		require.NoError(t, err)
		return WrapESClient(nil, nil, version, v8)
	}
	oli, err := elastic.NewSimpleClient(elastic.SetURL(url))
	require.NoError(t, err)
	return WrapESClient(oli, nil, version, nil)
}
