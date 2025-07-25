// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
    "context"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/internal/config"
	"github.com/jaegertracing/jaeger/internal/metrics"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
)

func TestElasticsearchFactory(t *testing.T) {
	f := NewFactory()
	f.coreFactory = getTestingFactoryBase(t, &escfg.Configuration{})
	f.metricsFactory = metrics.NullFactory
	v, command := config.Viperize(f.AddFlags)
	command.ParseFlags([]string{})
	f.InitFromViper(v, zap.NewNop())
	_, err := f.CreateSpanReader()
	require.NoError(t, err)

	_, err = f.CreateSpanWriter()
	require.NoError(t, err)

	_, err = f.CreateDependencyReader()
	require.NoError(t, err)

	_, err = f.CreateSamplingStore(1)
	require.NoError(t, err)

	require.NoError(t, f.Close())
}

func TestInheritSettingsFrom(t *testing.T) {
	primaryConfig := escfg.Configuration{
		MaxDocCount: 99,
	}
	primaryFactory := NewFactory()
	primaryFactory.Options.Config.Configuration = primaryConfig
	archiveConfig := escfg.Configuration{
		SendGetBodyAs: "PUT",
	}
	archiveFactory := NewFactory()
	archiveFactory.Options = NewOptions(archiveNamespace)
	archiveFactory.Options.Config.Configuration = archiveConfig
	archiveFactory.InheritSettingsFrom(primaryFactory)
	require.Equal(t, "PUT", archiveFactory.getConfig().SendGetBodyAs)
	require.Equal(t, 99, archiveFactory.getConfig().MaxDocCount)
}

func TestArchiveFactory(t *testing.T) {
	tests := []struct {
		name               string
		args               []string
		expectedReadAlias  string
		expectedWriteAlias string
	}{
		{
			name:               "default settings",
			args:               []string{},
			expectedReadAlias:  "archive",
			expectedWriteAlias: "archive",
		},
		{
			name:               "use read write aliases",
			args:               []string{"--es-archive.use-aliases=true"},
			expectedReadAlias:  "archive-read",
			expectedWriteAlias: "archive-write",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := NewArchiveFactory()
			v, command := config.Viperize(f.AddFlags)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Write(mockEsServerResponse)
			}))
			t.Cleanup(server.Close)
			serverArg := "--es-archive.server-urls=" + server.URL
			testArgs := append(test.args, serverArg)
			command.ParseFlags(testArgs)
			f.InitFromViper(v, zap.NewNop())
			err := f.Initialize(metrics.NullFactory, zaptest.NewLogger(t))
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, f.Close())
			})
			require.Equal(t, test.expectedReadAlias, f.Options.GetConfig().ReadAliasSuffix)
			require.Equal(t, test.expectedWriteAlias, f.Options.GetConfig().WriteAliasSuffix)
			require.True(t, f.Options.Config.UseReadWriteAliases)
			require.Equal(t, DefaultConfig().BulkProcessing, f.Options.GetConfig().BulkProcessing)
		})
	}
}


func TestFactoryBase_ESClientFailure(t *testing.T) {
	t.Run("newClientFn returns error immediately", func(t *testing.T) {
		f := &FactoryBase{
			newClientFn: func(_ context.Context, _ *escfg.Configuration, _ *zap.Logger, _ metrics.Factory) (es.Client, error) {
				return nil, errors.New("mock connection error")
			},
			logger:         zaptest.NewLogger(t),
			metricsFactory: metrics.NullFactory,
			config: &escfg.Configuration{
				Servers:            []string{"http://fake-es:9200"},
				CreateIndexTemplates: true,
			},
		}

		
		client, err := f.newClientFn(context.Background(), f.config, f.logger, f.metricsFactory)
		require.Nil(t, client)
		require.ErrorContains(t, err, "mock connection error")
	})
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
			factory := &Factory{
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

func getTestingFactoryBase(t *testing.T, cfg *escfg.Configuration) *FactoryBase {
	f := &FactoryBase{}
	err := SetFactoryForTest(f, zaptest.NewLogger(t), metrics.NullFactory, cfg)
	require.NoError(t, err)
	return f
}
