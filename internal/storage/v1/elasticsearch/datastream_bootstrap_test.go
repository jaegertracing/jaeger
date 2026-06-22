// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
