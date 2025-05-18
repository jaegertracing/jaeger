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
	"go.opentelemetry.io/otel"
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

func TestCreateTemplates(t *testing.T) {
	tests := []struct {
		err                    string
		spanTemplateService    func() *mocks.TemplateCreateService
		serviceTemplateService func() *mocks.TemplateCreateService
		indexPrefix            escfg.IndexPrefix
	}{
		{
			spanTemplateService: func() *mocks.TemplateCreateService {
				tService := &mocks.TemplateCreateService{}
				tService.On("Body", mock.Anything).Return(tService)
				tService.On("Do", context.Background()).Return(nil, nil)
				return tService
			},
			serviceTemplateService: func() *mocks.TemplateCreateService {
				tService := &mocks.TemplateCreateService{}
				tService.On("Body", mock.Anything).Return(tService)
				tService.On("Do", context.Background()).Return(nil, nil)
				return tService
			},
		},
		{
			spanTemplateService: func() *mocks.TemplateCreateService {
				tService := &mocks.TemplateCreateService{}
				tService.On("Body", mock.Anything).Return(tService)
				tService.On("Do", context.Background()).Return(nil, nil)
				return tService
			},
			serviceTemplateService: func() *mocks.TemplateCreateService {
				tService := &mocks.TemplateCreateService{}
				tService.On("Body", mock.Anything).Return(tService)
				tService.On("Do", context.Background()).Return(nil, nil)
				return tService
			},
			indexPrefix: "test",
		},
		{
			err: "span-template-error",
			spanTemplateService: func() *mocks.TemplateCreateService {
				tService := new(mocks.TemplateCreateService)
				tService.On("Body", mock.Anything).Return(tService)
				tService.On("Do", context.Background()).Return(nil, errors.New("span-template-error"))
				return tService
			},
			serviceTemplateService: func() *mocks.TemplateCreateService {
				tService := new(mocks.TemplateCreateService)
				tService.On("Body", mock.Anything).Return(tService)
				tService.On("Do", context.Background()).Return(nil, nil)
				return tService
			},
		},
		{
			err: "service-template-error",
			spanTemplateService: func() *mocks.TemplateCreateService {
				tService := new(mocks.TemplateCreateService)
				tService.On("Body", mock.Anything).Return(tService)
				tService.On("Do", context.Background()).Return(nil, nil)
				return tService
			},
			serviceTemplateService: func() *mocks.TemplateCreateService {
				tService := new(mocks.TemplateCreateService)
				tService.On("Body", mock.Anything).Return(tService)
				tService.On("Do", context.Background()).Return(nil, errors.New("service-template-error"))
				return tService
			},
		},
	}

	for _, test := range tests {
		f := FactoryBase{}
		mockClient := &mocks.Client{}
		f.newClientFn = func(_ *escfg.Configuration, _ *zap.Logger, _ metrics.Factory) (es.Client, error) {
			return mockClient, nil
		}
		f.logger = zaptest.NewLogger(t)
		f.metricsFactory = metrics.NullFactory
		f.config = &escfg.Configuration{CreateIndexTemplates: true, Indices: escfg.Indices{IndexPrefix: test.indexPrefix}}
		f.tracer = otel.GetTracerProvider()
		client, err := f.newClientFn(&escfg.Configuration{}, zaptest.NewLogger(t), metrics.NullFactory)
		require.NoError(t, err)
		f.client.Store(&client)
		f.templateBuilder = es.TextTemplateBuilder{}
		jaegerSpanId := test.indexPrefix.Apply("jaeger-span")
		jaegerServiceId := test.indexPrefix.Apply("jaeger-service")
		mockClient.On("CreateTemplate", jaegerSpanId).Return(test.spanTemplateService())
		mockClient.On("CreateTemplate", jaegerServiceId).Return(test.serviceTemplateService())
		err = f.createTemplates(context.Background())
		if test.err != "" {
			require.Error(t, err, test.err)
		}
	}
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
	factory, err := NewFactoryBase(context.Background(), cfg, metrics.NullFactory, zap.NewNop())
	require.NoError(t, err)
	defer factory.Close()
}

func TestESStorageFactoryWithConfigError(t *testing.T) {
	defer testutils.VerifyGoLeaksOnce(t)

	cfg := escfg.Configuration{
		Servers:  []string{"http://127.0.0.1:65535"},
		LogLevel: "error",
	}
	_, err := NewFactoryBase(context.Background(), cfg, metrics.NullFactory, zap.NewNop())
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

	cfg := escfg.Configuration{
		Servers:  []string{server.URL},
		LogLevel: "debug",
		Authentication: escfg.Authentication{
			BasicAuthentication: escfg.BasicAuthentication{
				PasswordFilePath: pwdFile,
			},
		},
	}

	logger, buf := testutils.NewEchoLogger(t)
	f, err := NewFactoryBase(context.Background(), cfg, metrics.NullFactory, logger)
	require.NoError(t, err)
	defer f.Close()

	f.config.Servers = []string{}
	f.onPasswordChange()
	assert.Contains(t, buf.String(), "no servers specified")

	require.NoError(t, os.Remove(pwdFile))
	f.onPasswordChange()
}
