// Copyright (c) 2025 The Jaeger Authors.
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
