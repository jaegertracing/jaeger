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
	"io/ioutil"
	"os"
	"testing"

	"github.com/flosch/pongo2/v4"
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
	f.InitFromViper(v)

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

func TestFactory_LoadMapping(t *testing.T) {
	tests := []struct {
		name     string
		esPrefix string
		useILM   bool
	}{
		{name: "/jaeger-span.json"},
		{name: "/jaeger-service.json"},
		{name: "/jaeger-span.json"},
		{name: "/jaeger-service.json"},
		{name: "/jaeger-span-7.json", esPrefix: "test", useILM: true},
		{name: "/jaeger-service-7.json", esPrefix: "test", useILM: true},
		{name: "/jaeger-dependencies.json"},
		{name: "/jaeger-dependencies-7.json"},
	}
	for _, test := range tests {
		mapping := loadMapping(test.name)
		f, err := os.Open("mappings/" + test.name)
		require.NoError(t, err)
		b, err := ioutil.ReadAll(f)
		require.NoError(t, err)
		assert.Equal(t, string(b), mapping)
		tempMapping, err := pongo2.FromString(mapping)
		assert.NoError(t, err)
		esPrefixTemplateVal := test.esPrefix
		if esPrefixTemplateVal != "" {
			esPrefixTemplateVal += "-"
		}
		expectedMapping, err := tempMapping.Execute(pongo2.Context{"NumberOfShards": 10, "NumberOfReplicas": 0, "ESPrefix": esPrefixTemplateVal, "UseILM": test.useILM})
		assert.NoError(t, err)
		actualMapping, err := fixMapping(es.PongoTemplateBuilder{}, mapping, 10, 0, test.esPrefix, test.useILM)
		assert.NoError(t, err)
		assert.Equal(t, expectedMapping, actualMapping)
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

func TestNewOptions(t *testing.T) {
	primaryCfg := escfg.Configuration{IndexPrefix: "primary"}
	archiveCfg := escfg.Configuration{IndexPrefix: "archive"}
	o := NewOptionsFromConfig(primaryCfg, archiveCfg)
	assert.Equal(t, primaryCfg, o.Primary.Configuration)
	assert.Equal(t, primaryNamespace, o.Primary.namespace)
	assert.Equal(t, 1, len(o.others))
	assert.Equal(t, archiveCfg, o.others[archiveNamespace].Configuration)
	assert.Equal(t, archiveNamespace, o.others[archiveNamespace].namespace)
}

func TestFixMapping(t *testing.T) {
	tests := []struct {
		name                    string
		templateBuilderMockFunc func() *mocks.TemplateBuilder
		expectedResult          string
		err                     string
	}{
		{name: "templateRenderSuccess",
			templateBuilderMockFunc: func() *mocks.TemplateBuilder {
				tb := mocks.TemplateBuilder{}
				ta := mocks.TemplateApplier{}
				ta.On("Execute", mock.Anything).Return("test success", nil)
				tb.On("FromString", mock.Anything).Return(&ta, nil)
				return &tb
			},
			expectedResult: "test success",
			err:            "",
		},
		{name: "templateRenderFailure",
			templateBuilderMockFunc: func() *mocks.TemplateBuilder {
				tb := mocks.TemplateBuilder{}
				ta := mocks.TemplateApplier{}
				ta.On("Execute", mock.Anything).Return("", errors.New("template exec error"))
				tb.On("FromString", mock.Anything).Return(&ta, nil)
				return &tb
			},
			expectedResult: "",
			err:            "template exec error",
		},
		{name: "templateLoadError",
			templateBuilderMockFunc: func() *mocks.TemplateBuilder {
				tb := mocks.TemplateBuilder{}
				tb.On("FromString", mock.Anything).Return(nil, errors.New("template load error"))
				return &tb
			},
			expectedResult: "",
			err:            "template load error",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := fixMapping(test.templateBuilderMockFunc(), "test", 3, 5, "test", true)
			assert.Equal(t, test.expectedResult, result)
			if test.err != "" {
				assert.EqualError(t, err, test.err)
			}

		})
	}
}
