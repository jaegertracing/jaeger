// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mappings

import (
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/mocks"
)

func newDataStreamBuilder(version es.BackendVersion) *MappingBuilder {
	opts := config.IndexOptions{Shards: 3, Replicas: new(int64(2))}
	return &MappingBuilder{
		TemplateBuilder: es.TextTemplateBuilder{},
		Indices: config.Indices{
			Spans:        opts,
			Services:     opts,
			Dependencies: opts,
			Sampling:     opts,
		},
		Version: version,
	}
}

// mappingBuilderReturning builds a MappingBuilder whose template rendering yields
// the given output (or fails), to exercise SpanDataStreamMappings' parse branches.
func mappingBuilderReturning(output string, parseErr, execErr error) *MappingBuilder {
	tb := &mocks.TemplateBuilder{}
	if parseErr != nil {
		tb.On("Parse", mock.Anything).Return(nil, parseErr)
	} else {
		ta := &mocks.TemplateApplier{}
		ta.On("Execute", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			if output != "" {
				_, _ = args.Get(0).(io.Writer).Write([]byte(output))
			}
		}).Return(execErr)
		tb.On("Parse", mock.Anything).Return(ta, nil)
	}
	return &MappingBuilder{
		TemplateBuilder: tb,
		Indices:         config.Indices{Spans: config.IndexOptions{Shards: 3, Replicas: new(int64(2))}},
		Version:         es.ElasticV8,
	}
}

func TestSpanDataStreamMappings(t *testing.T) {
	for _, v := range []es.BackendVersion{es.ElasticV7, es.ElasticV8, es.OpenSearch2} {
		t.Run(v.String(), func(t *testing.T) {
			body, err := newDataStreamBuilder(v).SpanDataStreamMappings()
			require.NoError(t, err)
			var doc struct {
				Template struct {
					Mappings struct {
						Properties map[string]any `json:"properties"`
					} `json:"mappings"`
				} `json:"template"`
			}
			require.NoError(t, json.Unmarshal([]byte(body), &doc))
			ts, ok := doc.Template.Mappings.Properties["@timestamp"]
			require.True(t, ok, "@timestamp must be present for data stream partitioning")
			assert.Equal(t, map[string]any{"type": "date_nanos"}, ts)
			// The standard span fields must still be present (reused verbatim),
			// i.e. more than just the injected @timestamp.
			assert.Greater(t, len(doc.Template.Mappings.Properties), 1)
		})
	}
}

func TestSpanDataStreamMappings_Errors(t *testing.T) {
	tests := []struct {
		name   string
		mb     *MappingBuilder
		errMsg string
	}{
		{"get mapping fails", mappingBuilderReturning("", errors.New("boom"), nil), "boom"},
		{"not json", mappingBuilderReturning("not-json", nil, nil), "failed to parse span mapping"},
		{"missing mappings object", mappingBuilderReturning(`{}`, nil, nil), "missing the 'mappings' object"},
		{"missing properties object", mappingBuilderReturning(`{"mappings":{}}`, nil, nil), "missing the 'mappings.properties' object"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.mb.SpanDataStreamMappings()
			require.ErrorContains(t, err, tt.errMsg)
		})
	}
}

func TestSpanDataStreamSettings(t *testing.T) {
	mb := newDataStreamBuilder(es.ElasticV8)

	withILM, err := mb.SpanDataStreamSettings(true, "my-policy")
	require.NoError(t, err)
	assert.Contains(t, withILM, `"index.lifecycle.name":"my-policy"`)
	assert.Contains(t, withILM, `"index.number_of_shards":3`)
	assert.Contains(t, withILM, `"index.number_of_replicas":2`)

	noILM, err := mb.SpanDataStreamSettings(false, "my-policy")
	require.NoError(t, err)
	assert.NotContains(t, noILM, "index.lifecycle.name")
}

func TestSpanDataStreamIndexTemplate(t *testing.T) {
	// Elasticsearch: references @custom via ignore_missing_component_templates.
	esTmpl, err := SpanDataStreamIndexTemplate("jaeger.spans", false)
	require.NoError(t, err)
	var esDoc map[string]any
	require.NoError(t, json.Unmarshal([]byte(esTmpl), &esDoc))
	assert.Equal(t, []any{"jaeger.spans"}, esDoc["index_patterns"])
	assert.Contains(t, esDoc, "data_stream")
	assert.InDelta(t, float64(500), esDoc["priority"], 0)
	assert.Equal(t, []any{"jaeger.spans@mappings", "jaeger.spans@settings", "jaeger.spans@custom"}, esDoc["composed_of"])
	assert.Equal(t, []any{"jaeger.spans@custom"}, esDoc["ignore_missing_component_templates"])

	// OpenSearch: omits @custom and ignore_missing_component_templates.
	osTmpl, err := SpanDataStreamIndexTemplate("jaeger.spans", true)
	require.NoError(t, err)
	var osDoc map[string]any
	require.NoError(t, json.Unmarshal([]byte(osTmpl), &osDoc))
	assert.Equal(t, []any{"jaeger.spans@mappings", "jaeger.spans@settings"}, osDoc["composed_of"])
	assert.NotContains(t, osDoc, "ignore_missing_component_templates")
}

func TestSpanDataStreamISMPolicy(t *testing.T) {
	body, err := newDataStreamBuilder(es.OpenSearch2).SpanDataStreamISMPolicy("prod.jaeger.spans")
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &doc))
	// The ism_template index pattern must be rendered with the data stream name.
	assert.Contains(t, body, `"prod.jaeger.spans"`)
}

func TestSpanDataStreamISMPolicy_Errors(t *testing.T) {
	_, err := mappingBuilderReturning("", errors.New("parse boom"), nil).SpanDataStreamISMPolicy("ds")
	require.ErrorContains(t, err, "parse boom")
	_, err = mappingBuilderReturning("", nil, errors.New("exec boom")).SpanDataStreamISMPolicy("ds")
	require.ErrorContains(t, err, "exec boom")
}

func TestSpanDataStreamILMPolicy(t *testing.T) {
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(SpanDataStreamILMPolicy()), &doc))
	assert.Contains(t, doc, "policy")
}
