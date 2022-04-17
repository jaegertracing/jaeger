// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package es

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/es"
	escfg "github.com/jaegertracing/jaeger/pkg/es/config"
	"github.com/jaegertracing/jaeger/pkg/es/mocks"
	"github.com/jaegertracing/jaeger/storage"
)

var _ storage.Factory = new(Factory)

type mockClientBuilder struct {
	escfg.Configuration
	err                 error
	createTemplateError error
}

func (m *mockClientBuilder) NewClient(logger *zap.Logger, metricsFactory metrics.Factory) (es.Client, error) {
	if m.err == nil {
		c := &mocks.Client{}
		tService := &mocks.TemplateCreateService{}
		tService.On("Body", mock.Anything).Return(tService)
		tService.On("Do", context.Background()).Return(nil, m.createTemplateError)
		c.On("CreateTemplate", mock.Anything).Return(tService)
		c.On("GetVersion").Return(uint(6))
		return c, nil
	}
	return nil, m.err
}

func TestElasticsearchFactory(t *testing.T) {
	f := NewFactory()
	v, command := config.Viperize(f.AddFlags)
	command.ParseFlags([]string{})
	f.InitFromViper(v, zap.NewNop())

	// after InitFromViper, f.primaryConfig points to a real session builder that will fail in unit tests,
	// so we override it with a mock.
	f.primaryConfig = &mockClientBuilder{err: errors.New("made-up error")}
	assert.EqualError(t, f.Initialize(metrics.NullFactory, zap.NewNop()), "failed to create primary Elasticsearch client: made-up error")

	f.primaryConfig = &mockClientBuilder{}
	f.archiveConfig = &mockClientBuilder{err: errors.New("made-up error2"), Configuration: escfg.Configuration{Enabled: true}}
	assert.EqualError(t, f.Initialize(metrics.NullFactory, zap.NewNop()), "failed to create archive Elasticsearch client: made-up error2")

	f.archiveConfig = &mockClientBuilder{}
	assert.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))

	_, err := f.CreateSpanReader()
	assert.NoError(t, err)

	_, err = f.CreateSpanWriter()
	assert.NoError(t, err)

	_, err = f.CreateDependencyReader()
	assert.NoError(t, err)

	_, err = f.CreateArchiveSpanReader()
	assert.NoError(t, err)

	_, err = f.CreateArchiveSpanWriter()
	assert.NoError(t, err)
	assert.NoError(t, f.Close())
}

func TestElasticsearchTagsFileDoNotExist(t *testing.T) {
	f := NewFactory()
	mockConf := &mockClientBuilder{}
	mockConf.Tags.File = "fixtures/tags_foo.txt"
	f.primaryConfig = mockConf
	f.archiveConfig = mockConf
	assert.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
	r, err := f.CreateSpanWriter()
	require.Error(t, err)
	assert.Nil(t, r)
}

func TestElasticsearchILMUsedWithoutReadWriteAliases(t *testing.T) {
	f := NewFactory()
	mockConf := &mockClientBuilder{}
	mockConf.UseILM = true
	f.primaryConfig = mockConf
	f.archiveConfig = mockConf
	assert.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
	w, err := f.CreateSpanWriter()
	require.EqualError(t, err, "--es.use-ilm must always be used in conjunction with --es.use-aliases to ensure ES writers and readers refer to the single index mapping")
	assert.Nil(t, w)

	r, err := f.CreateSpanReader()
	require.EqualError(t, err, "--es.use-ilm must always be used in conjunction with --es.use-aliases to ensure ES writers and readers refer to the single index mapping")
	assert.Nil(t, r)
}

func TestTagKeysAsFields(t *testing.T) {
	tests := []struct {
		path          string
		include       string
		expected      []string
		errorExpected bool
	}{
		{
			path:          "fixtures/do_not_exists.txt",
			errorExpected: true,
		},
		{
			path:     "fixtures/tags_01.txt",
			expected: []string{"foo", "bar", "space"},
		},
		{
			path:     "fixtures/tags_02.txt",
			expected: nil,
		},
		{
			include:  "televators,eriatarka,thewidow",
			expected: []string{"televators", "eriatarka", "thewidow"},
		},
		{
			expected: nil,
		},
		{
			path:     "fixtures/tags_01.txt",
			include:  "televators,eriatarka,thewidow",
			expected: []string{"foo", "bar", "space", "televators", "eriatarka", "thewidow"},
		},
		{
			path:     "fixtures/tags_02.txt",
			include:  "televators,eriatarka,thewidow",
			expected: []string{"televators", "eriatarka", "thewidow"},
		},
	}

	for _, test := range tests {
		cfg := escfg.Configuration{
			Tags: escfg.TagsAsFields{
				File:    test.path,
				Include: test.include,
			},
		}

		tags, err := cfg.TagKeysAsFields()
		if test.errorExpected {
			require.Error(t, err)
			assert.Nil(t, tags)
		} else {
			require.NoError(t, err)
			assert.Equal(t, test.expected, tags)
		}
	}
}

func TestCreateTemplateError(t *testing.T) {
	f := NewFactory()
	f.primaryConfig = &mockClientBuilder{createTemplateError: errors.New("template-error"), Configuration: escfg.Configuration{Enabled: true, CreateIndexTemplates: true}}
	f.archiveConfig = &mockClientBuilder{}
	err := f.Initialize(metrics.NullFactory, zap.NewNop())
	require.NoError(t, err)
	w, err := f.CreateSpanWriter()
	assert.Nil(t, w)
	assert.Error(t, err, "template-error")
}

func TestILMDisableTemplateCreation(t *testing.T) {
	f := NewFactory()
	f.primaryConfig = &mockClientBuilder{createTemplateError: errors.New("template-error"), Configuration: escfg.Configuration{Enabled: true, UseILM: true, UseReadWriteAliases: true, CreateIndexTemplates: true}}
	f.archiveConfig = &mockClientBuilder{}
	err := f.Initialize(metrics.NullFactory, zap.NewNop())
	require.NoError(t, err)
	_, err = f.CreateSpanWriter()
	assert.Nil(t, err) // as the createTemplate is not called, CreateSpanWriter should not return an error
}

func TestArchiveDisabled(t *testing.T) {
	f := NewFactory()
	f.archiveConfig = &mockClientBuilder{Configuration: escfg.Configuration{Enabled: false}}
	w, err := f.CreateArchiveSpanWriter()
	assert.Nil(t, w)
	assert.Nil(t, err)
	r, err := f.CreateArchiveSpanReader()
	assert.Nil(t, r)
	assert.Nil(t, err)
}

func TestArchiveEnabled(t *testing.T) {
	f := NewFactory()
	f.primaryConfig = &mockClientBuilder{}
	f.archiveConfig = &mockClientBuilder{Configuration: escfg.Configuration{Enabled: true}}
	err := f.Initialize(metrics.NullFactory, zap.NewNop())
	require.NoError(t, err)
	w, err := f.CreateArchiveSpanWriter()
	require.NoError(t, err)
	assert.NotNil(t, w)
	r, err := f.CreateArchiveSpanReader()
	require.NoError(t, err)
	assert.NotNil(t, r)
}

func TestInitFromOptions(t *testing.T) {
	f := NewFactory()
	o := Options{Primary: namespaceConfig{Configuration: escfg.Configuration{Servers: []string{"server"}}},
		others: map[string]*namespaceConfig{"es-archive": {Configuration: escfg.Configuration{Servers: []string{"server2"}}}}}
	f.InitFromOptions(o)
	assert.Equal(t, o.GetPrimary(), f.primaryConfig)
	assert.Equal(t, o.Get(archiveNamespace), f.archiveConfig)
}
