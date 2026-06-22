// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mappings

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

func dataStreamMappingBuilder() *MappingBuilder {
	replicas := int64(2)
	return &MappingBuilder{
		TemplateBuilder: es.TextTemplateBuilder{},
		Indices: config.Indices{
			Spans: config.IndexOptions{Shards: 5, Replicas: &replicas, Priority: 500},
		},
		EsVersion: 8,
	}
}

func TestSpanDataStreamMappings(t *testing.T) {
	mb := dataStreamMappingBuilder()
	got, err := mb.SpanDataStreamMappings()
	require.NoError(t, err)

	var doc struct {
		Template struct {
			Mappings struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"mappings"`
		} `json:"template"`
	}
	require.NoError(t, json.Unmarshal([]byte(got), &doc), "must be valid component-template JSON")

	// @timestamp (date_nanos) must be present for the data stream machinery.
	require.Contains(t, doc.Template.Mappings.Properties, "@timestamp")
	assert.JSONEq(t, `{"type":"date_nanos"}`, string(doc.Template.Mappings.Properties["@timestamp"]))
	// Standard span fields must be carried over verbatim (reuse, not reinvention).
	assert.Contains(t, doc.Template.Mappings.Properties, "traceID")
	assert.Contains(t, doc.Template.Mappings.Properties, "startTime")
}

func TestSpanDataStreamSettings(t *testing.T) {
	mb := dataStreamMappingBuilder()

	t.Run("opensearch (no lifecycle ref)", func(t *testing.T) {
		got, err := mb.SpanDataStreamSettings(false, "jaeger-spans-policy")
		require.NoError(t, err)
		var doc struct {
			Template struct {
				Settings map[string]any `json:"settings"`
			} `json:"template"`
		}
		require.NoError(t, json.Unmarshal([]byte(got), &doc))
		assert.EqualValues(t, 5, doc.Template.Settings["index.number_of_shards"])
		assert.EqualValues(t, 2, doc.Template.Settings["index.number_of_replicas"])
		assert.NotContains(t, doc.Template.Settings, "index.lifecycle.name")
	})

	t.Run("elasticsearch (ILM ref)", func(t *testing.T) {
		got, err := mb.SpanDataStreamSettings(true, "jaeger-spans-policy")
		require.NoError(t, err)
		var doc struct {
			Template struct {
				Settings map[string]any `json:"settings"`
			} `json:"template"`
		}
		require.NoError(t, json.Unmarshal([]byte(got), &doc))
		assert.Equal(t, "jaeger-spans-policy", doc.Template.Settings["index.lifecycle.name"])
	})
}

func TestSpanDataStreamIndexTemplate(t *testing.T) {
	type indexTemplate struct {
		IndexPatterns                   []string       `json:"index_patterns"`
		DataStream                      map[string]any `json:"data_stream"`
		ComposedOf                      []string       `json:"composed_of"`
		Priority                        int            `json:"priority"`
		IgnoreMissingComponentTemplates []string       `json:"ignore_missing_component_templates"`
	}

	t.Run("elasticsearch references optional @custom", func(t *testing.T) {
		got, err := SpanDataStreamIndexTemplate("prod.jaeger.spans", false)
		require.NoError(t, err)
		var doc indexTemplate
		require.NoError(t, json.Unmarshal([]byte(got), &doc))

		assert.Equal(t, []string{"prod.jaeger.spans"}, doc.IndexPatterns)
		assert.NotNil(t, doc.DataStream, "data_stream directive must be present")
		assert.Equal(t, []string{
			"prod.jaeger.spans@mappings",
			"prod.jaeger.spans@settings",
			"prod.jaeger.spans@custom",
		}, doc.ComposedOf)
		assert.Equal(t, 500, doc.Priority)
		assert.Equal(t, []string{"prod.jaeger.spans@custom"}, doc.IgnoreMissingComponentTemplates)
	})

	t.Run("opensearch omits @custom and ignore_missing", func(t *testing.T) {
		// OpenSearch does not support ignore_missing_component_templates, so @custom
		// is not referenced (it cannot be made optional there).
		got, err := SpanDataStreamIndexTemplate("jaeger.spans", true)
		require.NoError(t, err)
		var doc indexTemplate
		require.NoError(t, json.Unmarshal([]byte(got), &doc))

		assert.Equal(t, []string{"jaeger.spans@mappings", "jaeger.spans@settings"}, doc.ComposedOf)
		assert.Nil(t, doc.IgnoreMissingComponentTemplates)
		assert.NotContains(t, got, "@custom")
	})
}

func TestSpanDataStreamISMPolicy(t *testing.T) {
	mb := dataStreamMappingBuilder()
	got, err := mb.SpanDataStreamISMPolicy("jaeger.spans")
	require.NoError(t, err)

	var doc struct {
		Policy struct {
			DefaultState string `json:"default_state"`
			ISMTemplate  []struct {
				IndexPatterns []string `json:"index_patterns"`
			} `json:"ism_template"`
		} `json:"policy"`
	}
	require.NoError(t, json.Unmarshal([]byte(got), &doc), "must be valid ISM policy JSON")
	assert.Equal(t, "hot", doc.Policy.DefaultState)
	require.Len(t, doc.Policy.ISMTemplate, 1)
	assert.Equal(t, []string{"jaeger.spans"}, doc.Policy.ISMTemplate[0].IndexPatterns)
}

func TestSpanDataStreamILMPolicy(t *testing.T) {
	var doc struct {
		Policy struct {
			Phases map[string]json.RawMessage `json:"phases"`
		} `json:"policy"`
	}
	require.NoError(t, json.Unmarshal([]byte(SpanDataStreamILMPolicy()), &doc), "must be valid ILM policy JSON")
	assert.Contains(t, doc.Policy.Phases, "hot")
	assert.Contains(t, doc.Policy.Phases, "delete")
}
