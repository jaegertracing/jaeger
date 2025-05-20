// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/internal/config"
	"github.com/jaegertracing/jaeger/internal/metrics"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

const (
	pwd1 = "first password"
	pwd2 = "second password"
	// and with user name
	upwd1 = "user:" + pwd1
	upwd2 = "user:" + pwd2
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

func TestFactoryInitializeErr(t *testing.T) {
	tests := []struct {
		name        string
		factory     *Factory
		expectedErr string
	}{
		{
			name:        "cfg validation err",
			factory:     &Factory{Options: &Options{Config: namespaceConfig{Configuration: escfg.Configuration{}}}},
			expectedErr: "Servers: non zero value required",
		},
		{
			name:        "server error",
			factory:     NewFactory(),
			expectedErr: "failed to create Elasticsearch client: health check timeout: Head \"http://127.0.0.1:9200\": dial tcp 127.0.0.1:9200: connect: connection refused: no Elasticsearch node available",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.factory.Initialize(metrics.NullFactory, zaptest.NewLogger(t))
			require.EqualError(t, err, test.expectedErr)
		})
	}
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

func getTestingFactory(t *testing.T, pwdFile string, server *httptest.Server) *Factory {
	require.NoError(t, os.WriteFile(pwdFile, []byte(pwd1), 0o600))
	cfg := escfg.Configuration{
		Servers:  []string{server.URL},
		LogLevel: "debug",
		Authentication: escfg.Authentication{
			BasicAuthentication: escfg.BasicAuthentication{
				Username:         "user",
				PasswordFilePath: pwdFile,
			},
		},
		BulkProcessing: escfg.BulkProcessing{
			MaxBytes: -1, // disable bulk; we want immediate flush
		},
	}
	coreFactory := &FactoryBase{}
	f := NewArchiveFactory()
	f.coreFactory = coreFactory
	options := &Options{}
	options.Config = namespaceConfig{Configuration: cfg}
	f.Options = options
	require.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
	t.Cleanup(func() {
		require.NoError(t, f.Close())
	})
	return f
}

func getTestingServer(t *testing.T) (*httptest.Server, *sync.Map) {
	authReceived := &sync.Map{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("request to fake ES server: %v", r)
		// epecting header in the form Authorization:[Basic OmZpcnN0IHBhc3N3b3Jk]
		h := strings.Split(r.Header.Get("Authorization"), " ")
		if !assert.Len(t, h, 2) {
			return
		}
		assert.Equal(t, "Basic", h[0])
		authBytes, err := base64.StdEncoding.DecodeString(h[1])
		assert.NoError(t, err, "header: %s", h)
		auth := string(authBytes)
		authReceived.Store(auth, auth)
		t.Logf("request to fake ES server contained auth=%s", auth)
		w.Write(mockEsServerResponse)
	}))
	t.Cleanup(func() {
		server.Close()
	})
	return server, authReceived
}

func getTestingFactoryBase(t *testing.T, cfg *escfg.Configuration) *FactoryBase {
	f := &FactoryBase{}
	err := SetFactoryForTest(f, zaptest.NewLogger(t), metrics.NullFactory, cfg)
	require.NoError(t, err)
	return f
}
