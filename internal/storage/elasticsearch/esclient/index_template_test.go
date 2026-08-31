// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/featuregate"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

func TestMappingTypeFromString(t *testing.T) {
	tests := []struct {
		input    string
		expected MappingType
		wantErr  bool
	}{
		{config.SpanIndexName, SpanMapping, false},
		{config.ServiceIndexName, ServiceMapping, false},
		{config.DependencyIndexName, DependencyMapping, false},
		{config.SamplingIndexName, SamplingMapping, false},
		{"not-a-mapping", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := MappingTypeFromString(tt.input)
			if tt.wantErr {
				require.ErrorContains(t, err, "invalid mapping type")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
			// String round-trips back to the index base name it was resolved from.
			assert.Equal(t, tt.input, got.String())
		})
	}
}

func TestRenderIndexTemplateUnknownType(t *testing.T) {
	_, err := RenderIndexTemplate(MappingType(99), config.Indices{}, false, "", es.ElasticV8)
	require.ErrorContains(t, err, "unknown index template mapping type")
}

func TestRenderIndexTemplateNilReplicas(t *testing.T) {
	// config.IndexOptions.Replicas is a pointer; a caller that builds Indices without
	// defaults must get a clear error rather than a nil-dereference panic.
	_, err := RenderIndexTemplate(SpanMapping, config.Indices{}, false, "", es.ElasticV8)
	require.ErrorContains(t, err, "no replica count configured")
}

func TestRenderIndexTemplateInvalidJSON(t *testing.T) {
	// A prefix carrying a double quote makes the rendered template invalid JSON
	// (the prefix appears in the ILM alias name), exercising the parse-failure branch.
	indices := config.Indices{
		Spans:       config.IndexOptions{Replicas: new(int64)},
		IndexPrefix: `bad"prefix-`,
	}
	_, err := RenderIndexTemplate(SpanMapping, indices, true, "policy", es.ElasticV8)
	require.ErrorContains(t, err, "not valid JSON")
}

// setTypedAttributeIndexing flips the gate for the duration of a test.
func setTypedAttributeIndexing(t *testing.T, enabled bool) {
	original := TypedAttributeIndexingGate.IsEnabled()
	require.NoError(t, featuregate.GlobalRegistry().Set(TypedAttributeIndexingGate.ID(), enabled))
	t.Cleanup(func() {
		require.NoError(t, featuregate.GlobalRegistry().Set(TypedAttributeIndexingGate.ID(), original))
	})
}

// dig walks a rendered template by dot-separated path, where a numeric segment
// indexes into an array.
func dig(t *testing.T, doc any, path string) any {
	t.Helper()
	for _, segment := range strings.Split(path, ".") {
		if index, err := strconv.Atoi(segment); err == nil {
			array, ok := doc.([]any)
			require.True(t, ok, "%s: expected an array at %q", path, segment)
			require.Less(t, index, len(array), "%s: index %d out of range", path, index)
			doc = array[index]
			continue
		}
		object, ok := doc.(map[string]any)
		require.True(t, ok, "%s: expected an object at %q", path, segment)
		doc, ok = object[segment]
		require.True(t, ok, "%s: no key %q", path, segment)
	}
	return doc
}

// renderSpanMapping renders the span template and returns its "mappings" object.
// The ES7 envelope keeps the rendered body at the top level, and the mapping body
// itself does not vary by version, so one version is enough to inspect it.
func renderSpanMapping(t *testing.T) any {
	t.Helper()
	indices := config.Indices{Spans: config.IndexOptions{Shards: 5, Replicas: new(int64)}}
	rendered, err := RenderIndexTemplate(SpanMapping, indices, false, "", es.ElasticV7)
	require.NoError(t, err)
	var doc any
	require.NoError(t, json.Unmarshal([]byte(rendered), &doc))
	return dig(t, doc, "mappings")
}

// The paths that carry an attribute value Jaeger can be asked to order on: the
// span, resource, and event levels, in both the nested and the elevated
// representation.
var typedAttributeValuePaths = []string{
	"dynamic_templates.0.span_tags_map.mapping",
	"dynamic_templates.1.process_tags_map.mapping",
	"properties.tags.properties.value",
	"properties.process.properties.tags.properties.value",
	"properties.logs.properties.fields.properties.value",
}

// The paths deliberately left as bare keywords. Link attributes are out of scope
// (RFC 0015 §6), and scopeTag/scopeTags are unused remnants rather than a level
// anything writes to.
var untypedAttributeValuePaths = []string{
	"dynamic_templates.2.scope_tags_map.mapping",
	"properties.references.properties.tags.properties.value",
	"properties.scopeTags.properties.value",
}

func TestRenderSpanTemplateTypedAttributesDisabled(t *testing.T) {
	setTypedAttributeIndexing(t, false)
	mappings := renderSpanMapping(t)
	for _, path := range append(typedAttributeValuePaths, untypedAttributeValuePaths...) {
		value, ok := dig(t, mappings, path).(map[string]any)
		require.True(t, ok, path)
		assert.Equal(t, "keyword", value["type"], path)
		assert.NotContains(t, value, "fields", path)
	}
}

func TestRenderSpanTemplateTypedAttributesEnabled(t *testing.T) {
	setTypedAttributeIndexing(t, true)
	mappings := renderSpanMapping(t)

	for _, path := range typedAttributeValuePaths {
		value, ok := dig(t, mappings, path).(map[string]any)
		require.True(t, ok, path)
		// The keyword the value is already indexed as is untouched, so `eq` and
		// the 256-character bound behave exactly as before.
		assert.Equal(t, "keyword", value["type"], path)
		assert.InDelta(t, 256, value["ignore_above"], 0, path)

		// The numeric sub-field is the only one: a boolean sub-field cannot be
		// mapped on both engines, and the keyword already answers the equality a
		// boolean gets (see the template's own note).
		assert.Equal(t, map[string]any{
			"number": map[string]any{
				"type":             "double",
				"coerce":           false,
				"ignore_malformed": true,
			},
		}, dig(t, value, "fields"), path)
	}

	for _, path := range untypedAttributeValuePaths {
		value, ok := dig(t, mappings, path).(map[string]any)
		require.True(t, ok, path)
		assert.NotContains(t, value, "fields", path)
	}
}

func TestRenderIndexTemplateTypedAttributesValidForAllVersions(t *testing.T) {
	// The sub-fields are appended after "ignore_above", so the rendered body's
	// comma placement is what a malformed conditional would break first, and
	// RenderIndexTemplate reports that as invalid JSON.
	setTypedAttributeIndexing(t, true)
	indices := config.Indices{
		Spans:        config.IndexOptions{Shards: 5, Replicas: new(int64)},
		Services:     config.IndexOptions{Shards: 5, Replicas: new(int64)},
		Dependencies: config.IndexOptions{Shards: 5, Replicas: new(int64)},
		Sampling:     config.IndexOptions{Shards: 5, Replicas: new(int64)},
	}
	mappings := []MappingType{SpanMapping, ServiceMapping, DependencyMapping, SamplingMapping}
	for _, version := range es.AllVersions {
		for _, mapping := range mappings {
			t.Run(version.String()+"/"+mapping.String(), func(t *testing.T) {
				for _, useILM := range []bool{false, true} {
					_, err := RenderIndexTemplate(mapping, indices, useILM, "policy", version)
					require.NoError(t, err)
				}
			})
		}
	}
}
