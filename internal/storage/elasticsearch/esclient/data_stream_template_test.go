// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/snapshottest"
)

// decodeComponentTemplate decodes the "template" envelope of a rendered component
// body.
func decodeComponentTemplate(t *testing.T, body string) map[string]any {
	t.Helper()
	var decoded struct {
		Template map[string]any `json:"template"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &decoded))
	return decoded.Template
}

// TestSpanDataStreamComponentsMatchRotationTemplate is the drift guard. Both
// component bodies are derived from jaeger-span.json rather than restated, and this
// pins that derivation: "@settings" must carry the span rotation template's settings
// verbatim, and "@mappings" its mappings plus the one "@timestamp" field a data
// stream adds. It fails if the two write paths are ever rendered from different
// inputs — a different template, different render flags, or a field added to one
// component but not to the index it derives from.
func TestSpanDataStreamComponentsMatchRotationTemplate(t *testing.T) {
	indices := testIndices()
	body, err := renderSpanIndexTemplateBody(indices)
	require.NoError(t, err)
	templates, err := spanDataStreamTemplates("jaeger.spans", body)
	require.NoError(t, err)
	require.Len(t, templates, 3)

	// On ES7 the rotation template renders the neutral body at the top level, so its
	// "settings" and "mappings" are directly comparable to the two components.
	rotation, err := RenderIndexTemplate(SpanMapping, indices, false, "", es.ElasticV7)
	require.NoError(t, err)
	var want struct {
		Settings map[string]any `json:"settings"`
		Mappings map[string]any `json:"mappings"`
	}
	require.NoError(t, json.Unmarshal([]byte(rotation), &want))

	gotSettings := decodeComponentTemplate(t, templates[1].body)["settings"]
	assert.Equal(t, want.Settings, gotSettings,
		"@settings must equal the span rotation template's settings")

	gotMappings, ok := decodeComponentTemplate(t, templates[0].body)["mappings"].(map[string]any)
	require.True(t, ok)
	properties, ok := gotMappings["properties"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, map[string]any{"type": "date_nanos"}, properties["@timestamp"],
		"data streams require an @timestamp field mapped as date_nanos")
	delete(properties, "@timestamp")
	assert.Equal(t, want.Mappings, gotMappings,
		"@mappings must equal the span rotation template's mappings plus @timestamp")
}

func TestParseSpanIndexTemplateBodyErrors(t *testing.T) {
	t.Run("invalid json", func(t *testing.T) {
		_, err := parseSpanIndexTemplateBody([]byte("not-json"))
		require.ErrorContains(t, err, "failed to parse span index template")
	})

	t.Run("no settings", func(t *testing.T) {
		_, err := parseSpanIndexTemplateBody([]byte(`{"mappings":{}}`))
		require.ErrorContains(t, err, "no settings object")
	})
}

func TestSpanDataStreamTemplatesErrors(t *testing.T) {
	settings := json.RawMessage(`{}`)

	t.Run("mappings without properties", func(t *testing.T) {
		_, err := spanDataStreamTemplates("jaeger.spans", spanIndexTemplateBody{
			Settings: settings,
			Mappings: map[string]any{},
		})
		require.ErrorContains(t, err, "no properties object")
	})

	t.Run("mappings that cannot be marshaled", func(t *testing.T) {
		// A channel has no JSON representation, so the component body fails to
		// marshal rather than silently emitting a truncated template.
		_, err := spanDataStreamTemplates("jaeger.spans", spanIndexTemplateBody{
			Settings: settings,
			Mappings: map[string]any{"properties": map[string]any{"bad": make(chan int)}},
		})
		require.ErrorContains(t, err, "failed to marshal span data stream mappings")
	})

	t.Run("settings that cannot be marshaled", func(t *testing.T) {
		_, err := spanDataStreamTemplates("jaeger.spans", spanIndexTemplateBody{
			Settings: json.RawMessage("{not-json"),
			Mappings: map[string]any{"properties": map[string]any{}},
		})
		require.ErrorContains(t, err, "failed to marshal span data stream settings")
	})
}

func TestCreateDataStreamTemplates(t *testing.T) {
	t.Run("nil replicas errors before any request", func(t *testing.T) {
		rec, url := okServer(t)
		c := IndicesClient{Client: makeClient(t, url, "", ""), Indices: config.Indices{}}
		err := c.CreateDataStreamTemplates(context.Background())
		require.ErrorContains(t, err, "no replica count configured")
		assert.Empty(t, rec.Requests(), "a render failure must not issue any request")
	})

	t.Run("server error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(esErrResponse))
		}))
		defer srv.Close()
		c := IndicesClient{Client: makeClient(t, srv.URL, "", ""), Indices: testIndices()}
		err := c.CreateDataStreamTemplates(context.Background())
		require.ErrorContains(t, err, `failed to create data stream template "jaeger.spans@mappings"`)
	})

	t.Run("transport error", func(t *testing.T) {
		c := IndicesClient{Client: makeClient(t, "http://localhost:1", "", ""), Indices: testIndices()}
		err := c.CreateDataStreamTemplates(context.Background())
		require.ErrorContains(t, err, `failed to create data stream template "jaeger.spans@mappings"`)
	})
}

// TestCreateDataStreamTemplatesRequestSnapshot freezes the exact bytes of the three
// PUTs that back the span data stream, in the order they are issued (ADR-012
// §Wire-format stability). Recording every backend version and letting
// AssertByVersion collapse them also asserts that this path is version-invariant:
// unlike CreateTemplate it must not branch on UsesV8API, so a single all-versions
// snapshot is the expected outcome and a per-version split would fail the test.
func TestCreateDataStreamTemplatesRequestSnapshot(t *testing.T) {
	content := map[es.BackendVersion]string{}
	for _, version := range es.AllVersions {
		rec, url := okServer(t)
		// UseILM is deliberately left unset: the data-stream templates never render
		// the rotation path's lifecycle settings, whatever the client is configured
		// with. templateSnapshotIndices carries the "test-" prefix, so the snapshot
		// also pins its resolution to the dot-notation base "test.jaeger.spans".
		c := IndicesClient{
			Client:  makeClient(t, url, "", "", version),
			Indices: templateSnapshotIndices(),
		}
		require.NoError(t, c.CreateDataStreamTemplates(context.Background()))
		content[version] = rec.Marshal(t)
	}
	snapshottest.AssertByVersion(t, "testdata/create_data_stream_templates", content)
}
