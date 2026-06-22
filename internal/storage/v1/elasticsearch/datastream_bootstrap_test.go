// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.uber.org/zap"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/mappings"
)

type fakeTemplates struct {
	components     map[string]string
	indexTemplates map[string]string
	err            error
}

func newFakeTemplates() *fakeTemplates {
	return &fakeTemplates{components: map[string]string{}, indexTemplates: map[string]string{}}
}

func (f *fakeTemplates) CreateComponentTemplate(name, template string) error {
	if f.err != nil {
		return f.err
	}
	f.components[name] = template
	return nil
}

func (f *fakeTemplates) CreateIndexTemplate(name, template string) error {
	if f.err != nil {
		return f.err
	}
	f.indexTemplates[name] = template
	return nil
}

type fakeLifecycle struct {
	exists     bool
	existsErr  error
	createErr  error
	createdFor []string
}

func (f *fakeLifecycle) Exists(string) (bool, error) { return f.exists, f.existsErr }

func (f *fakeLifecycle) Create(name, _ string) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.createdFor = append(f.createdFor, name)
	return nil
}

func bootstrapMappingBuilder() *mappings.MappingBuilder {
	replicas := int64(1)
	return &mappings.MappingBuilder{
		TemplateBuilder: es.TextTemplateBuilder{},
		Indices:         config.Indices{Spans: config.IndexOptions{Shards: 5, Replicas: &replicas}},
		EsVersion:       8,
	}
}

func newBootstrap(tpl componentTemplateCreator, lc lifecyclePolicyManager, useILM bool) dataStreamBootstrap {
	return dataStreamBootstrap{
		templates:      tpl,
		lifecycle:      lc,
		mappingBuilder: bootstrapMappingBuilder(),
		dataStreamName: "jaeger.spans",
		policyName:     "jaeger-spans-policy",
		policyBody:     `{"policy":{}}`,
		useILM:         useILM,
	}
}

func TestDataStreamBootstrap_CreatesAllObjects(t *testing.T) {
	tpl := newFakeTemplates()
	lc := &fakeLifecycle{exists: false}

	require.NoError(t, newBootstrap(tpl, lc, false).run())

	// Policy created (was absent), both component templates, and the index template.
	assert.Equal(t, []string{"jaeger-spans-policy"}, lc.createdFor)
	assert.Contains(t, tpl.components, "jaeger.spans@mappings")
	assert.Contains(t, tpl.components, "jaeger.spans@settings")
	assert.Contains(t, tpl.indexTemplates, "jaeger.spans")
}

func TestDataStreamBootstrap_PolicyExists_NotOverwritten(t *testing.T) {
	tpl := newFakeTemplates()
	lc := &fakeLifecycle{exists: true}

	require.NoError(t, newBootstrap(tpl, lc, true).run())

	// Existing policy must not be recreated, but templates are still applied.
	assert.Empty(t, lc.createdFor)
	assert.Len(t, tpl.components, 2)
	assert.Contains(t, tpl.indexTemplates, "jaeger.spans")
}

func TestDataStreamBootstrap_PropagatesErrors(t *testing.T) {
	t.Run("lifecycle exists error", func(t *testing.T) {
		err := newBootstrap(newFakeTemplates(), &fakeLifecycle{existsErr: errors.New("boom")}, false).run()
		require.ErrorContains(t, err, "failed to check lifecycle policy")
	})
	t.Run("lifecycle create error", func(t *testing.T) {
		err := newBootstrap(newFakeTemplates(), &fakeLifecycle{exists: false, createErr: errors.New("boom")}, false).run()
		require.ErrorContains(t, err, "failed to create lifecycle policy")
	})
	t.Run("template create error", func(t *testing.T) {
		err := newBootstrap(&fakeTemplates{err: errors.New("boom")}, &fakeLifecycle{exists: true}, false).run()
		require.Error(t, err)
	})
}

func TestDataStreamPolicyBody(t *testing.T) {
	mb := bootstrapMappingBuilder()

	t.Run("elasticsearch uses built-in ILM policy", func(t *testing.T) {
		body, err := dataStreamPolicyBody(&config.DataStreamRotation{}, false, "jaeger.spans", mb)
		require.NoError(t, err)
		assert.Contains(t, body, "phases")
	})
	t.Run("opensearch uses built-in ISM policy", func(t *testing.T) {
		body, err := dataStreamPolicyBody(&config.DataStreamRotation{}, true, "jaeger.spans", mb)
		require.NoError(t, err)
		assert.Contains(t, body, "ism_template")
	})
	t.Run("policy file overrides built-in", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "policy.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"custom":true}`), 0o600))
		body, err := dataStreamPolicyBody(&config.DataStreamRotation{PolicyFile: path}, true, "jaeger.spans", mb)
		require.NoError(t, err)
		assert.JSONEq(t, `{"custom":true}`, body)
	})
	t.Run("missing policy file is an error", func(t *testing.T) {
		_, err := dataStreamPolicyBody(&config.DataStreamRotation{PolicyFile: "/no/such/file.json"}, true, "jaeger.spans", mb)
		require.ErrorContains(t, err, "failed to read data stream policy file")
	})
}

func TestBootstrapSpanDataStream_OpenSearch(t *testing.T) {
	var puts []string
	server := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/":
			res.Write([]byte(`{"version":{"number":"3.7.0"},"tagline":"The OpenSearch Project"}`))
		case req.Method == http.MethodGet: // ISM policy existence check
			res.WriteHeader(http.StatusNotFound)
		case req.Method == http.MethodPut:
			puts = append(puts, req.URL.Path)
			res.WriteHeader(http.StatusCreated)
		default:
			res.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	replicas := int64(1)
	cfg := &config.Configuration{
		Servers: []string{server.URL},
		Version: 7,
		Indices: config.Indices{
			Spans: config.IndexOptions{
				Shards:   1,
				Replicas: &replicas,
				Rotation: config.RotationConfig{DataStream: configoptional.Some(config.DataStreamRotation{})},
			},
		},
	}
	f := &FactoryBase{config: cfg, logger: zap.NewNop(), templateBuilder: es.TextTemplateBuilder{}}
	mb := f.mappingBuilderFromConfig(cfg)

	require.NoError(t, f.bootstrapSpanDataStream(context.Background(), &mb))

	// ISM policy, both component templates, and the index template must be created.
	assert.Contains(t, puts, "/_plugins/_ism/policies/jaeger-spans-policy")
	assert.Contains(t, puts, "/_component_template/jaeger.spans@mappings")
	assert.Contains(t, puts, "/_component_template/jaeger.spans@settings")
	assert.Contains(t, puts, "/_index_template/jaeger.spans")
}

func TestBootstrapSpanDataStream_FlavorError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, _ *http.Request) {
		res.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	replicas := int64(1)
	cfg := &config.Configuration{
		Servers: []string{server.URL},
		Version: 7,
		Indices: config.Indices{
			Spans: config.IndexOptions{
				Shards:   1,
				Replicas: &replicas,
				Rotation: config.RotationConfig{DataStream: configoptional.Some(config.DataStreamRotation{})},
			},
		},
	}
	f := &FactoryBase{config: cfg, logger: zap.NewNop(), templateBuilder: es.TextTemplateBuilder{}}
	mb := f.mappingBuilderFromConfig(cfg)
	err := f.bootstrapSpanDataStream(context.Background(), &mb)
	require.ErrorContains(t, err, "failed to detect Elasticsearch/OpenSearch flavor")
}
