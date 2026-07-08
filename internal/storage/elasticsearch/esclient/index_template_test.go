// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
