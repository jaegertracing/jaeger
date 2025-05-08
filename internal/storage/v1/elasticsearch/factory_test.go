// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
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
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/internal/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/mocks"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

var mockEsServerResponse = []byte(`
{
	"Version": {
		"Number": "6"
	}
}
`)

type mockClientBuilder struct {
	err                 error
	createTemplateError error
}

func (m *mockClientBuilder) NewClient(*escfg.Configuration, *zap.Logger, metrics.Factory) (es.Client, error) {
	if m.err == nil {
		c := &mocks.Client{}
		tService := &mocks.TemplateCreateService{}
		tService.On("Body", mock.Anything).Return(tService)
		tService.On("Do", context.Background()).Return(nil, m.createTemplateError)
		c.On("CreateTemplate", mock.Anything).Return(tService)
		c.On("GetVersion").Return(uint(6))
		c.On("Close").Return(nil)
		return c, nil
	}
	return nil, m.err
}

func TestElasticsearchTagsFileDoNotExist(t *testing.T) {
	f := NewFactoryBase()
	f.config = &escfg.Configuration{
		Tags: escfg.TagsAsFields{
			File: "fixtures/file-does-not-exist.txt",
		},
	}
	f.newClientFn = (&mockClientBuilder{}).NewClient
	require.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
	defer f.Close()
	r, err := f.GetSpanWriterParams()
	require.ErrorContains(t, err, "open fixtures/file-does-not-exist.txt: no such file or directory")
	assert.Empty(t, r)
}

func TestElasticsearchILMUsedWithoutReadWriteAliases(t *testing.T) {
	f := NewFactoryBase()
	f.config = &escfg.Configuration{
		UseILM: true,
	}
	f.newClientFn = (&mockClientBuilder{}).NewClient
	require.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
	defer f.Close()
	w, err := f.GetSpanWriterParams()
	require.EqualError(t, err, "--es.use-ilm must always be used in conjunction with --es.use-aliases to ensure ES writers and readers refer to the single index mapping")
	assert.Empty(t, w)

	r, err := f.GetSpanReaderParams()
	require.EqualError(t, err, "--es.use-ilm must always be used in conjunction with --es.use-aliases to ensure ES writers and readers refer to the single index mapping")
	assert.Empty(t, r)
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
	f := NewFactoryBase()
	f.config = &escfg.Configuration{CreateIndexTemplates: true}
	f.newClientFn = (&mockClientBuilder{createTemplateError: errors.New("template-error")}).NewClient
	err := f.Initialize(metrics.NullFactory, zap.NewNop())
	require.NoError(t, err)
	defer f.Close()

	s, err := f.CreateSamplingStore(1)
	assert.Nil(t, s)
	require.Error(t, err, "template-error")
}

func TestESStorageFactoryWithConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(mockEsServerResponse)
	}))
	defer server.Close()
	cfg := escfg.Configuration{
		Servers:  []string{server.URL},
		LogLevel: "error",
	}
	factory, err := NewFactoryBaseWithConfig(cfg, metrics.NullFactory, zap.NewNop())
	require.NoError(t, err)
	defer factory.Close()
}

func TestConfigurationValidation(t *testing.T) {
	testCases := []struct {
		name    string
		cfg     escfg.Configuration
		wantErr bool
	}{
		{
			name: "valid configuration",
			cfg: escfg.Configuration{
				Servers: []string{"http://localhost:9200"},
			},
			wantErr: false,
		},
		{
			name:    "missing servers",
			cfg:     escfg.Configuration{},
			wantErr: true,
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			err := test.cfg.Validate()
			if test.wantErr {
				require.Error(t, err)
				_, err = NewFactoryBaseWithConfig(test.cfg, metrics.NullFactory, zap.NewNop())
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestESStorageFactoryWithConfigError(t *testing.T) {
	defer testutils.VerifyGoLeaksOnce(t)

	cfg := escfg.Configuration{
		Servers:  []string{"http://127.0.0.1:65535"},
		LogLevel: "error",
	}
	_, err := NewFactoryBaseWithConfig(cfg, metrics.NullFactory, zap.NewNop())
	require.ErrorContains(t, err, "failed to create Elasticsearch client")
}

func TestFactoryESClientsAreNil(t *testing.T) {
	f := &FactoryBase{}
	assert.Nil(t, f.getClient())
}

func TestPasswordFromFileErrors(t *testing.T) {
	defer testutils.VerifyGoLeaksOnce(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(mockEsServerResponse)
	}))
	defer server.Close()

	pwdFile := filepath.Join(t.TempDir(), "pwd")
	require.NoError(t, os.WriteFile(pwdFile, []byte("first password"), 0o600))

	f := NewFactoryBase()
	f.config = &escfg.Configuration{
		Servers:  []string{server.URL},
		LogLevel: "debug",
		Authentication: escfg.Authentication{
			BasicAuthentication: escfg.BasicAuthentication{
				PasswordFilePath: pwdFile,
			},
		},
	}

	logger, buf := testutils.NewEchoLogger(t)
	require.NoError(t, f.Initialize(metrics.NullFactory, logger))
	defer f.Close()

	f.config.Servers = []string{}
	f.onPasswordChange()
	assert.Contains(t, buf.String(), "no servers specified")

	require.NoError(t, os.Remove(pwdFile))
	f.onPasswordChange()
}

func TestIsArchiveCapable(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		enabled   bool
		expected  bool
	}{
		{
			name:      "archive capable",
			namespace: "es-archive",
			enabled:   true,
			expected:  true,
		},
		{
			name:      "not capable",
			namespace: "es-archive",
			enabled:   false,
			expected:  false,
		},
		{
			name:      "capable + wrong namespace",
			namespace: "es",
			enabled:   true,
			expected:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factory := &FactoryBase{
				Options: &Options{
					Config: namespaceConfig{
						namespace: test.namespace,
						Configuration: escfg.Configuration{
							Enabled: test.enabled,
						},
					},
				},
			}
			result := factory.IsArchiveCapable()
			require.Equal(t, test.expected, result)
		})
	}
}

func getTestingFactoryBase(t *testing.T) *FactoryBase {
	f := NewFactoryBase()
	f.newClientFn = (&mockClientBuilder{}).NewClient
	f.logger = zaptest.NewLogger(t)
	f.metricsFactory = metrics.NullFactory
	f.config = &escfg.Configuration{}
	return f
}
