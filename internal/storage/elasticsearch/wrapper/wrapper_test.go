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

func TestClientWrapper_CreateComposableTemplate_AcceptsCreated(t *testing.T) {
	// A 201 Created (not just 200 OK) must be treated as success on the v8 path.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := newTestWrapper(t, es.ElasticV8, srv.URL)
	require.NoError(t, c.CreateComponentTemplate(context.Background(), "jaeger-span-mappings", "{}"))
	require.NoError(t, c.CreateComposableIndexTemplate(context.Background(), "jaeger-span", "{}"))
}

func TestClientWrapper_CreateComposableTemplates_Failures(t *testing.T) {
	for _, version := range []es.BackendVersion{es.ElasticV7, es.ElasticV8} {
		t.Run(version.String(), func(t *testing.T) {
			errSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-Elastic-Product", "Elasticsearch")
				w.WriteHeader(http.StatusBadRequest)
			}))
			defer errSrv.Close()
			c := newTestWrapper(t, version, errSrv.URL)

			// A non-2xx response surfaces as an error.
			require.Error(t, c.CreateComponentTemplate(context.Background(), "x", "{}"))
			require.Error(t, c.CreateComposableIndexTemplate(context.Background(), "x", "{}"))

			// A canceled context aborts the request on both backends.
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			require.Error(t, c.CreateComponentTemplate(ctx, "x", "{}"))
			require.Error(t, c.CreateComposableIndexTemplate(ctx, "x", "{}"))

			// A transport error (server unreachable) surfaces as an error.
			downSrv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			downURL := downSrv.URL
			downSrv.Close()
			require.Error(t, newTestWrapper(t, version, downURL).CreateComponentTemplate(context.Background(), "x", "{}"))
		})
	}
}

func TestClientWrapper_CreateLifecyclePolicy(t *testing.T) {
	const policy = `{"policy":{"phases":{}}}`
	tests := []struct {
		name     string
		version  es.BackendVersion
		wantPath string // ILM on ES, ISM on OpenSearch
	}{
		{"ilm / v7 (olivere)", es.ElasticV7, "/_ilm/policy/jaeger-spans-policy"},
		{"ilm / v8", es.ElasticV8, "/_ilm/policy/jaeger-spans-policy"},
		{"ism / opensearch (olivere)", es.OpenSearch2, "/_plugins/_ism/policies/jaeger-spans-policy"},
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

			c := newTestWrapper(t, tt.version, srv.URL)
			require.NoError(t, c.CreateLifecyclePolicy(context.Background(), "jaeger-spans-policy", policy))
			assert.Equal(t, http.MethodPut, gotMethod)
			assert.Equal(t, tt.wantPath, gotPath)
			assert.JSONEq(t, policy, gotBody)
		})
	}
}

func TestClientWrapper_LifecyclePolicyExists(t *testing.T) {
	tests := []struct {
		name       string
		version    es.BackendVersion
		status     int
		wantExists bool
		wantPath   string
	}{
		{"ilm v7 present", es.ElasticV7, http.StatusOK, true, "/_ilm/policy/jaeger-spans-policy"},
		{"ilm v7 absent", es.ElasticV7, http.StatusNotFound, false, "/_ilm/policy/jaeger-spans-policy"},
		{"ilm v8 present", es.ElasticV8, http.StatusOK, true, "/_ilm/policy/jaeger-spans-policy"},
		{"ilm v8 absent", es.ElasticV8, http.StatusNotFound, false, "/_ilm/policy/jaeger-spans-policy"},
		{"ism opensearch present", es.OpenSearch2, http.StatusOK, true, "/_plugins/_ism/policies/jaeger-spans-policy"},
		{"ism opensearch absent", es.OpenSearch2, http.StatusNotFound, false, "/_plugins/_ism/policies/jaeger-spans-policy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotMethod, gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath = r.Method, r.URL.Path
				w.Header().Set("X-Elastic-Product", "Elasticsearch")
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			c := newTestWrapper(t, tt.version, srv.URL)
			exists, err := c.LifecyclePolicyExists(context.Background(), "jaeger-spans-policy")
			require.NoError(t, err)
			assert.Equal(t, tt.wantExists, exists)
			assert.Equal(t, http.MethodGet, gotMethod)
			assert.Equal(t, tt.wantPath, gotPath)
		})
	}
}

func TestClientWrapper_LifecyclePolicy_Failures(t *testing.T) {
	for _, version := range []es.BackendVersion{es.ElasticV7, es.ElasticV8, es.OpenSearch2} {
		t.Run(version.String(), func(t *testing.T) {
			errSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-Elastic-Product", "Elasticsearch")
				w.WriteHeader(http.StatusBadRequest)
			}))
			defer errSrv.Close()
			c := newTestWrapper(t, version, errSrv.URL)

			// A non-2xx create surfaces as an error.
			require.Error(t, c.CreateLifecyclePolicy(context.Background(), "p", `{}`))
			// A 4xx other than 404 on the existence check surfaces as an error.
			_, err := c.LifecyclePolicyExists(context.Background(), "p")
			require.Error(t, err)

			// A transport error (server unreachable) surfaces on both calls.
			downSrv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			downURL := downSrv.URL
			downSrv.Close()
			down := newTestWrapper(t, version, downURL)
			require.Error(t, down.CreateLifecyclePolicy(context.Background(), "p", `{}`))
			_, err = down.LifecyclePolicyExists(context.Background(), "p")
			require.Error(t, err)
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
