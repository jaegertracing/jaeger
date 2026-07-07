// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

// canonicalJSON re-encodes a JSON string with sorted keys so two semantically
// equal documents compare equal regardless of whitespace or key order.
func canonicalJSON(t *testing.T, s string) string {
	t.Helper()
	var v any
	require.NoErrorf(t, json.Unmarshal([]byte(s), &v), "not valid JSON: %s", s)
	b, err := json.MarshalIndent(v, "", "  ")
	require.NoError(t, err)
	return string(b)
}

// TestRenderIndexTemplateMatchesLegacyMappings pins the esclient-owned rendering
// to the pre-M4b mappings output: for every mapping type and backend the wire
// body must be semantically identical to the committed golden fixture the v1
// mappings package rendered.
func TestRenderIndexTemplateMatchesLegacyMappings(t *testing.T) {
	ptr := func(v int64) *int64 { return &v }
	indices := config.Indices{
		IndexPrefix:  "test-",
		Spans:        config.IndexOptions{Shards: 3, Replicas: ptr(3), Priority: 500},
		Services:     config.IndexOptions{Shards: 3, Replicas: ptr(3), Priority: 501},
		Dependencies: config.IndexOptions{Shards: 3, Replicas: ptr(3), Priority: 502},
		Sampling:     config.IndexOptions{Shards: 3, Replicas: ptr(3), Priority: 503},
	}
	subjects := []struct {
		mapping MappingType
		name    string
	}{
		{SpanMapping, "span"},
		{ServiceMapping, "service"},
		{DependencyMapping, "dependencies"},
		{SamplingMapping, "sampling"},
	}
	variants := []struct {
		version es.BackendVersion
		file    string
	}{
		{es.ElasticV7, "es7"},
		{es.ElasticV8, "es8-9"},
		{es.OpenSearch2, "os1-3"},
	}
	for _, s := range subjects {
		for _, v := range variants {
			t.Run(s.name+"/"+v.file, func(t *testing.T) {
				got, err := renderIndexTemplate(s.mapping, indices, true, "jaeger-test-policy", v.version)
				require.NoError(t, err)
				want, err := os.ReadFile(filepath.Join(
					"..", "..", "v1", "elasticsearch", "mappings", "testdata", s.name+"."+v.file+".json"))
				require.NoError(t, err)
				assert.Equal(t, canonicalJSON(t, string(want)), canonicalJSON(t, got))
			})
		}
	}
}

func TestRenderIndexTemplateUnknownType(t *testing.T) {
	_, err := renderIndexTemplate(MappingType(99), config.Indices{}, false, "", es.ElasticV8)
	require.ErrorContains(t, err, "unknown index template mapping type")
}
