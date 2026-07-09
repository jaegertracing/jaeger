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
)

func TestRenderSpanDataStreamMappings(t *testing.T) {
	body, err := renderSpanDataStreamMappings()
	require.NoError(t, err)

	var got struct {
		Template struct {
			Mappings map[string]any `json:"mappings"`
		} `json:"template"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &got))

	properties := got.Template.Mappings["properties"].(map[string]any)
	assert.Equal(t, map[string]any{"type": "date_nanos"}, properties["@timestamp"],
		"data streams require an @timestamp field mapped as date_nanos")
	assert.Contains(t, properties, "traceID", "the span field mappings must be carried over")
}

func TestSpanMappingsComponentErrors(t *testing.T) {
	t.Run("invalid json", func(t *testing.T) {
		_, err := spanMappingsComponent([]byte("not-json"))
		require.ErrorContains(t, err, "failed to parse span mappings")
	})

	t.Run("mappings without properties", func(t *testing.T) {
		_, err := spanMappingsComponent([]byte(`{"mappings":{}}`))
		require.ErrorContains(t, err, "no properties object")
	})

	t.Run("mappings that cannot be marshaled", func(t *testing.T) {
		// A channel has no JSON representation, so the component body fails to
		// marshal rather than silently emitting a truncated template.
		_, err := mappingsComponentBody(map[string]any{
			"properties": map[string]any{"bad": make(chan int)},
		})
		require.ErrorContains(t, err, "failed to marshal span data stream mappings")
	})
}

// TestSpanDataStreamMappingsMatchRotationTemplate is the drift guard: the
// @mappings component must carry exactly the span rotation template's field
// mappings plus @timestamp, so the data stream never diverges from the index it
// derives from. It fails if a future edit changes jaeger-span.json's mappings
// without the data stream tracking it.
func TestSpanDataStreamMappingsMatchRotationTemplate(t *testing.T) {
	body, err := renderSpanDataStreamMappings()
	require.NoError(t, err)

	var component struct {
		Template struct {
			Mappings map[string]any `json:"mappings"`
		} `json:"template"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &component))
	dsMappings := component.Template.Mappings

	// @timestamp is the only field the data stream adds on top of the rotation
	// template; drop it before comparing the rest.
	properties := dsMappings["properties"].(map[string]any)
	require.Contains(t, properties, "@timestamp")
	delete(properties, "@timestamp")

	rotation, err := RenderIndexTemplate(SpanMapping, testIndices(), false, "", es.ElasticV7)
	require.NoError(t, err)
	var rotationBody struct {
		Mappings map[string]any `json:"mappings"`
	}
	require.NoError(t, json.Unmarshal([]byte(rotation), &rotationBody))

	assert.Equal(t, rotationBody.Mappings, dsMappings,
		"data-stream @mappings must equal the span rotation template's mappings plus @timestamp")
}

func TestRenderSpanDataStreamSettings(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		body, err := renderSpanDataStreamSettings(templateSnapshotIndices())
		require.NoError(t, err)
		var got struct {
			Template struct {
				Settings map[string]any `json:"settings"`
			} `json:"template"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &got))
		assert.EqualValues(t, 3, got.Template.Settings["index.number_of_shards"])
		assert.EqualValues(t, 3, got.Template.Settings["index.number_of_replicas"])
		assert.EqualValues(t, 50, got.Template.Settings["index.mapping.nested_fields.limit"])
		assert.Equal(t, true, got.Template.Settings["index.requests.cache.enable"])
		// The lifecycle reference belongs to the policy slice, not this one.
		assert.NotContains(t, got.Template.Settings, "index.lifecycle.name")
	})

	t.Run("nil replicas", func(t *testing.T) {
		_, err := renderSpanDataStreamSettings(config.Indices{})
		require.ErrorContains(t, err, "no replica count configured")
	})
}

func TestRenderSpanDataStreamIndexTemplate(t *testing.T) {
	var got struct {
		IndexPatterns                   []string       `json:"index_patterns"`
		DataStream                      map[string]any `json:"data_stream"`
		ComposedOf                      []string       `json:"composed_of"`
		Priority                        int            `json:"priority"`
		IgnoreMissingComponentTemplates []string       `json:"ignore_missing_component_templates"`
	}
	require.NoError(t, json.Unmarshal([]byte(renderSpanDataStreamIndexTemplate("prod.jaeger.spans")), &got))

	assert.Equal(t, []string{"prod.jaeger.spans"}, got.IndexPatterns)
	assert.NotNil(t, got.DataStream, "data_stream must be present as an empty object")
	assert.Empty(t, got.DataStream)
	assert.Equal(t, []string{
		"prod.jaeger.spans@mappings",
		"prod.jaeger.spans@settings",
		"prod.jaeger.spans@custom",
	}, got.ComposedOf)
	assert.Equal(t, 500, got.Priority)
	assert.Equal(t, []string{"prod.jaeger.spans@custom"}, got.IgnoreMissingComponentTemplates)
}

func TestCreateDataStreamTemplates(t *testing.T) {
	t.Run("creates components before the index template", func(t *testing.T) {
		rec, url := okServer(t)
		c := IndicesClient{Client: makeClient(t, url, "", ""), Indices: testIndices()}
		require.NoError(t, c.CreateDataStreamTemplates(context.Background()))

		reqs := rec.Requests()
		require.Len(t, reqs, 3)
		for _, r := range reqs {
			assert.Equal(t, http.MethodPut, r.Method)
		}
		assert.Equal(t, "/_component_template/jaeger.spans@mappings", reqs[0].Path)
		assert.Equal(t, "/_component_template/jaeger.spans@settings", reqs[1].Path)
		assert.Equal(t, "/_index_template/jaeger.spans", reqs[2].Path)
	})

	t.Run("resolves index prefix to dot notation", func(t *testing.T) {
		rec, url := okServer(t)
		// templateSnapshotIndices carries the "test-" prefix; it must resolve to the
		// dot-notation base "test.jaeger.spans" in every template name.
		c := IndicesClient{Client: makeClient(t, url, "", ""), Indices: templateSnapshotIndices()}
		require.NoError(t, c.CreateDataStreamTemplates(context.Background()))

		reqs := rec.Requests()
		require.Len(t, reqs, 3)
		assert.Equal(t, "/_component_template/test.jaeger.spans@mappings", reqs[0].Path)
		assert.Equal(t, "/_component_template/test.jaeger.spans@settings", reqs[1].Path)
		assert.Equal(t, "/_index_template/test.jaeger.spans", reqs[2].Path)
	})

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
