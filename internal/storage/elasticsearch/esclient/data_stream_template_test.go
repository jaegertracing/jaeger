// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

// freshClusterServer answers as a cluster that has no "@custom" component yet: the
// existence probe 404s and everything else succeeds. That is the fresh-install path,
// where Jaeger creates the component before composing it.
func freshClusterServer(t *testing.T) (*snapshottest.Recorder, string) {
	t.Helper()
	rec := snapshottest.NewRecorder(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, componentCustomSuffix) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"status":404}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	})
	server := httptest.NewServer(rec)
	t.Cleanup(server.Close)
	return rec, server.URL
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
	templates, err := renderSpanDataStreamTemplates(indices)
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

// TestRenderMappingsComponentDoesNotMutateInput pins that adding "@timestamp" stays
// inside the component body. renderNeutralBody is now shared with the rotation path,
// so a component built by mutating the rendered mappings in place would leak a
// data-stream-only field into whatever else reads that body.
func TestRenderMappingsComponentDoesNotMutateInput(t *testing.T) {
	raw := json.RawMessage(`{"properties":{"traceID":{"type":"keyword"}}}`)
	before := string(raw)

	body, err := renderMappingsComponent(raw)
	require.NoError(t, err)

	assert.JSONEq(t, before, string(raw), "the caller's rendered mappings must be untouched")
	assert.Contains(t, body, "@timestamp", "the component itself still carries the field")
}

func TestSpanDataStreamIndexTemplatePriority(t *testing.T) {
	decodePriority := func(t *testing.T, body string) int64 {
		t.Helper()
		var decoded struct {
			Priority int64 `json:"priority"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &decoded))
		return decoded.Priority
	}

	templates, err := renderSpanDataStreamTemplates(testIndices())
	require.NoError(t, err)
	assert.Equal(t, int64(dataStreamPriority), decodePriority(t, templates[2].body))

	// indices.spans.priority tunes the rotation templates, whose jaeger-span-* pattern
	// never competes with the exact name jaeger.spans, so a value set there must not
	// reach this template.
	tuned := testIndices()
	tuned.Spans.Priority = 42
	templates, err = renderSpanDataStreamTemplates(tuned)
	require.NoError(t, err)
	assert.Equal(t, int64(dataStreamPriority), decodePriority(t, templates[2].body))
}

// TestSpanDataStreamIndexTemplateComposesCustom pins the composed_of contract that
// the backend matrix forced: "@custom" is composed unconditionally and
// ignore_missing_component_templates is never emitted, because that field exists on
// no OpenSearch version and both parsers reject unknown fields.
func TestSpanDataStreamIndexTemplateComposesCustom(t *testing.T) {
	body, err := renderSpanDataStreamIndexTemplate("jaeger.spans")
	require.NoError(t, err)
	assert.NotContains(t, body, "ignore_missing_component_templates")

	var decoded struct {
		ComposedOf []string `json:"composed_of"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &decoded))
	assert.Equal(t, []string{
		"jaeger.spans@mappings",
		"jaeger.spans@settings",
		"jaeger.spans@custom",
	}, decoded.ComposedOf)
}

func TestSpanDataStreamTemplatesErrors(t *testing.T) {
	// A neutral body is decoded from a rendered template, so a missing half means
	// that template lost a field. Catching it here names the field, instead of
	// PUTting a component whose body is literally null.
	t.Run("neutral body missing a half", func(t *testing.T) {
		for _, field := range []string{"mappings", "settings"} {
			inner := map[string]json.RawMessage{
				"mappings": json.RawMessage(`{"properties":{}}`),
				"settings": json.RawMessage(`{}`),
			}
			delete(inner, field)
			_, err := spanDataStreamTemplates("jaeger.spans", inner)
			require.ErrorContains(t, err, "span index template has no "+field+" object")
		}
	})

	t.Run("mappings that are not valid JSON", func(t *testing.T) {
		_, err := spanDataStreamTemplates("jaeger.spans", map[string]json.RawMessage{
			"mappings": json.RawMessage("not-json"),
			"settings": json.RawMessage(`{}`),
		})
		require.ErrorContains(t, err, "failed to parse span index template mappings")
	})

	t.Run("mappings without properties", func(t *testing.T) {
		_, err := spanDataStreamTemplates("jaeger.spans", map[string]json.RawMessage{
			"mappings": json.RawMessage(`{}`),
			"settings": json.RawMessage(`{}`),
		})
		require.ErrorContains(t, err, "no properties object")
	})

	t.Run("settings that cannot be marshaled", func(t *testing.T) {
		// json.RawMessage marshals its bytes verbatim, so a malformed body fails the
		// call rather than PUTting an invalid settings component.
		_, err := spanDataStreamTemplates("jaeger.spans", map[string]json.RawMessage{
			"mappings": json.RawMessage(`{"properties":{}}`),
			"settings": json.RawMessage("{not-json"),
		})
		require.ErrorContains(t, err, "failed to marshal span data stream settings")
	})
}

func TestCreateSpanDataStreamTemplates(t *testing.T) {
	t.Run("nil replicas errors before any request", func(t *testing.T) {
		rec, url := okServer(t)
		c := IndicesClient{Client: makeClient(t, url, "", ""), Indices: config.Indices{}}
		err := c.CreateSpanDataStreamTemplates(context.Background())
		require.ErrorContains(t, err, "no replica count configured")
		assert.Empty(t, rec.Requests(), "a render failure must not issue any request")
	})

	t.Run("an existing @custom is left untouched", func(t *testing.T) {
		rec, url := okServer(t)
		c := IndicesClient{Client: makeClient(t, url, "", ""), Indices: testIndices()}
		require.NoError(t, c.CreateSpanDataStreamTemplates(context.Background()))
		for _, r := range rec.Requests() {
			if strings.Contains(r.Path, componentCustomSuffix) {
				assert.Equal(t, http.MethodGet, r.Method,
					"@custom is user-owned: probe it, never overwrite it")
			}
		}
	})

	t.Run("a missing @custom is created empty", func(t *testing.T) {
		rec, url := freshClusterServer(t)
		c := IndicesClient{Client: makeClient(t, url, "", ""), Indices: testIndices()}
		require.NoError(t, c.CreateSpanDataStreamTemplates(context.Background()))
		var created bool
		for _, r := range rec.Requests() {
			if r.Method == http.MethodPut && strings.Contains(r.Path, componentCustomSuffix) {
				created = true
				assert.JSONEq(t, emptyComponentBody, string(r.Body),
					"Jaeger declares @custom and leaves its contents to the user")
			}
		}
		assert.True(t, created, "a missing @custom must be created before it is composed")
	})

	// The two paths a conditional create can take when it is refused. The refusal
	// looks the same either way — a 400 — so what separates them is whether the
	// template exists afterwards.
	t.Run("a @custom created during the race is kept", func(t *testing.T) {
		var probes int
		rec := snapshottest.NewRecorder(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.URL.Path, componentCustomSuffix) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("{}"))
				return
			}
			// The user creates @custom between Jaeger's probe and its write: the
			// first probe misses it, the conditional create is refused, and the
			// second probe finds theirs.
			if r.Method == http.MethodGet {
				probes++
				if probes == 1 {
					w.WriteHeader(http.StatusNotFound)
					w.Write([]byte(`{"status":404}`))
					return
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("{}"))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":{"type":"illegal_argument_exception",` +
				`"reason":"component template [jaeger.spans@custom] already exists"},"status":400}`))
		})
		srv := httptest.NewServer(rec)
		defer srv.Close()
		c := IndicesClient{Client: makeClient(t, srv.URL, "", ""), Indices: testIndices()}
		require.NoError(t, c.CreateSpanDataStreamTemplates(context.Background()))
		for _, r := range rec.Requests() {
			if r.Method == http.MethodPut && strings.Contains(r.Path, componentCustomSuffix) {
				assert.Equal(t, []string{"true"}, r.Query["create"],
					"the write must be conditional, so the user's template survives it")
			}
		}
	})

	t.Run("a create that fails for a real reason is surfaced", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// @custom stays absent throughout, so the refusal is not a lost race.
			if r.Method == http.MethodGet {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"status":404}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(esErrResponse))
		}))
		defer srv.Close()
		c := IndicesClient{Client: makeClient(t, srv.URL, "", ""), Indices: testIndices()}
		err := c.CreateSpanDataStreamTemplates(context.Background())
		require.ErrorContains(t, err, `failed to create data stream template "jaeger.spans@custom"`)
	})

	t.Run("component probe error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(esErrResponse))
		}))
		defer srv.Close()
		c := IndicesClient{Client: makeClient(t, srv.URL, "", ""), Indices: testIndices()}
		err := c.CreateSpanDataStreamTemplates(context.Background())
		require.ErrorContains(t, err, "failed to check if component template")
	})

	t.Run("server error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("{}"))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(esErrResponse))
		}))
		defer srv.Close()
		c := IndicesClient{Client: makeClient(t, srv.URL, "", ""), Indices: testIndices()}
		err := c.CreateSpanDataStreamTemplates(context.Background())
		require.ErrorContains(t, err, `failed to create data stream template "jaeger.spans@mappings"`)
	})

	t.Run("transport error on the probe", func(t *testing.T) {
		c := IndicesClient{Client: makeClient(t, "http://localhost:1", "", ""), Indices: testIndices()}
		err := c.CreateSpanDataStreamTemplates(context.Background())
		require.ErrorContains(t, err, "failed to check if component template")
	})

	t.Run("transport error on a template PUT", func(t *testing.T) {
		// The probe succeeds and the connection is then dropped mid-PUT, so the
		// failure arrives as a transport error rather than an HTTP status — the one
		// path that does not carry a ResponseError to prefix.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("{}"))
				return
			}
			// Assertions belong on the test goroutine, not in a handler, so a
			// failed hijack just returns: the ErrorContains below is what fails.
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				return
			}
			conn.Close()
		}))
		defer srv.Close()
		c := IndicesClient{Client: makeClient(t, srv.URL, "", ""), Indices: testIndices()}
		err := c.CreateSpanDataStreamTemplates(context.Background())
		require.ErrorContains(t, err, `failed to create data stream template "jaeger.spans@mappings"`)
	})
}

func TestTestsOnlyDeleteSpanDataStreamObjects(t *testing.T) {
	t.Run("deletes the stream before the templates it composes", func(t *testing.T) {
		rec, url := okServer(t)
		c := IndicesClient{Client: makeClient(t, url, "", ""), Indices: testIndices()}
		require.NoError(t, c.TestsOnlyDeleteSpanDataStreamObjects(context.Background()))

		paths := make([]string, 0, len(rec.Requests()))
		for _, r := range rec.Requests() {
			assert.Equal(t, http.MethodDelete, r.Method)
			paths = append(paths, r.Path)
		}
		assert.Equal(t, []string{
			"/_data_stream/jaeger.spans",
			"/_index_template/jaeger.spans",
			"/_component_template/jaeger.spans@mappings",
			"/_component_template/jaeger.spans@settings",
			"/_component_template/jaeger.spans@custom",
		}, paths, "a template cannot be dropped while a stream composed from it exists")
	})

	t.Run("tolerates objects that are already absent", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"status":404}`))
		}))
		defer srv.Close()
		c := IndicesClient{Client: makeClient(t, srv.URL, "", ""), Indices: testIndices()}
		require.NoError(t, c.TestsOnlyDeleteSpanDataStreamObjects(context.Background()))
	})

	t.Run("surfaces a real failure", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(esErrResponse))
		}))
		defer srv.Close()
		c := IndicesClient{Client: makeClient(t, srv.URL, "", ""), Indices: testIndices()}
		err := c.TestsOnlyDeleteSpanDataStreamObjects(context.Background())
		require.ErrorContains(t, err, "failed to delete _data_stream/jaeger.spans")
	})
}

func TestTestsOnlyDataStreamExists(t *testing.T) {
	// dataStreamClient answers the existence probe with a fixed status and body.
	dataStreamClient := func(t *testing.T, status int, body string) (IndicesClient, *snapshottest.Recorder) {
		t.Helper()
		rec := snapshottest.NewRecorder(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			w.Write([]byte(body))
		})
		server := httptest.NewServer(rec)
		t.Cleanup(server.Close)
		return IndicesClient{
			Client:  makeClient(t, server.URL, "", ""),
			Indices: testIndices(),
		}, rec
	}

	t.Run("the stream is listed", func(t *testing.T) {
		c, rec := dataStreamClient(t, http.StatusOK, `{"data_streams":[{"name":"jaeger.spans"}]}`)
		exists, err := c.TestsOnlyDataStreamExists(context.Background(), "jaeger.spans")
		require.NoError(t, err)
		assert.True(t, exists)
		require.Len(t, rec.Requests(), 1)
		assert.Equal(t, http.MethodGet, rec.Requests()[0].Method)
		assert.Equal(t, "/_data_stream/jaeger.spans", rec.Requests()[0].Path)
	})

	// This is the case the probe exists for: an ordinary index of that name answers
	// 200 with no streams, which a status-code check would read as a data stream.
	t.Run("an ordinary index of that name is not a stream", func(t *testing.T) {
		c, _ := dataStreamClient(t, http.StatusOK, `{"data_streams":[]}`)
		exists, err := c.TestsOnlyDataStreamExists(context.Background(), "jaeger.spans")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("nothing of that name at all", func(t *testing.T) {
		c, _ := dataStreamClient(t, http.StatusNotFound, `{"status":404}`)
		exists, err := c.TestsOnlyDataStreamExists(context.Background(), "jaeger.spans")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("surfaces a real failure", func(t *testing.T) {
		c, _ := dataStreamClient(t, http.StatusInternalServerError, esErrResponse)
		_, err := c.TestsOnlyDataStreamExists(context.Background(), "jaeger.spans")
		require.ErrorContains(t, err, `failed to check if data stream "jaeger.spans" exists`)
	})

	t.Run("unparseable response", func(t *testing.T) {
		c, _ := dataStreamClient(t, http.StatusOK, "not-json")
		_, err := c.TestsOnlyDataStreamExists(context.Background(), "jaeger.spans")
		require.ErrorContains(t, err, `failed to parse the data stream response for "jaeger.spans"`)
	})
}

// TestCreateSpanDataStreamTemplatesRequestSnapshot freezes the exact bytes of the PUTs
// that back the span data stream, in the order they are issued (ADR-012 §Wire-format
// stability), against a cluster that does not yet have an "@custom" component — the
// fresh-install path. Recording every backend version and letting AssertByVersion
// collapse them also asserts that this path is version-invariant: unlike
// CreateTemplate it must not branch on UsesV8API, so a single all-versions snapshot
// is the expected outcome and a per-version split would fail the test.
func TestCreateSpanDataStreamTemplatesRequestSnapshot(t *testing.T) {
	content := map[es.BackendVersion]string{}
	for _, version := range es.AllVersions {
		rec, url := freshClusterServer(t)
		// UseILM is deliberately left unset: the data-stream templates never render
		// the rotation path's lifecycle settings, whatever the client is configured
		// with. templateSnapshotIndices carries the "test-" prefix, so the snapshot
		// also pins its resolution to the dot-notation base "test.jaeger.spans".
		c := IndicesClient{
			Client:  makeClient(t, url, "", "", version),
			Indices: templateSnapshotIndices(),
		}
		require.NoError(t, c.CreateSpanDataStreamTemplates(context.Background()))
		content[version] = rec.Marshal(t)
	}
	snapshottest.AssertByVersion(t, "testdata/create_data_stream_templates", content)
}
