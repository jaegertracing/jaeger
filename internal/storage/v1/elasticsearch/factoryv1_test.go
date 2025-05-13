// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/config"
	"github.com/jaegertracing/jaeger/internal/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/testutils"
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
	f.coreFactory = getTestingFactoryBase(t)
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

func TestCreateTemplateErr(t *testing.T) {
	tests := []struct {
		name      string
		cfg       *escfg.Configuration
		expectErr bool
	}{
		{
			name:      "error",
			cfg:       &escfg.Configuration{CreateIndexTemplates: true},
			expectErr: true,
		},
		{
			name: "ILMDisableTemplateCreation",
			cfg:  &escfg.Configuration{UseILM: true, UseReadWriteAliases: true, CreateIndexTemplates: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewFactory()
			f.coreFactory = getTestingFactoryBaseWithCreateTemplateError(t, tt.cfg, errors.New("template-error"))
			wr, err := f.CreateSpanWriter()
			if tt.expectErr {
				require.ErrorContains(t, err, "template-error")
				assert.Nil(t, wr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSpanWriterMappingBuilderErr(t *testing.T) {
	coreFactory := &FactoryBase{}
	err := SetFactoryForTestWithMappingErr(coreFactory, zaptest.NewLogger(t), metrics.NullFactory, &escfg.Configuration{CreateIndexTemplates: true}, errors.New("template-error"))
	require.NoError(t, err)
	f := NewFactory()
	f.coreFactory = coreFactory
	_, err = f.CreateSpanWriter()
	require.ErrorContains(t, err, "template-error")
}

func TestSpanReaderErr(t *testing.T) {
	f := NewFactory()
	f.coreFactory = getTestingFactoryBaseWithCreateTemplateError(t, &escfg.Configuration{UseILM: true, UseReadWriteAliases: false}, errors.New("template-error"))
	r, err := f.CreateSpanReader()
	require.ErrorContains(t, err, "--es.use-ilm must always be used in conjunction with --es.use-aliases to ensure ES writers and readers refer to the single index mapping")
	assert.Nil(t, r)
}

func TestPasswordFromFile(t *testing.T) {
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	t.Run("primary client", func(t *testing.T) {
		pwdFile := filepath.Join(t.TempDir(), "pwd")
		server, authReceived := getTestingServer(t)
		f := getTestingFactory(t, pwdFile, server)
		testPasswordFromFile(t, pwdFile, authReceived, f.coreFactory.getClient, f.CreateSpanWriter)
	})
	t.Run("load token error", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "does not exist")
		token, err := loadTokenFromFile(file)
		require.Error(t, err)
		assert.Empty(t, token)
	})
}

func TestInheritSettingsFrom(t *testing.T) {
	primaryConfig := &escfg.Configuration{
		MaxDocCount: 99,
	}
	primaryFactory := NewFactory()
	primaryFactory.coreFactory = getTestingFactoryBase(t)
	primaryFactory.coreFactory.SetConfig(primaryConfig)
	archiveConfig := &escfg.Configuration{
		SendGetBodyAs: "PUT",
	}
	archiveFactory := NewFactory()
	archiveFactory.Options = NewOptions(archiveNamespace)
	archiveFactory.coreFactory = getTestingFactoryBase(t)
	archiveFactory.coreFactory.SetConfig(archiveConfig)
	archiveFactory.InheritSettingsFrom(primaryFactory)
	require.Equal(t, "PUT", archiveFactory.coreFactory.GetConfig().SendGetBodyAs)
	require.Equal(t, 99, archiveFactory.coreFactory.GetConfig().MaxDocCount)
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

func testPasswordFromFile(t *testing.T, pwdFile string, authReceived *sync.Map, getClient func() es.Client, getWriter func() (spanstore.Writer, error)) {
	writer, err := getWriter()
	require.NoError(t, err)
	span := &model.Span{
		Process: &model.Process{ServiceName: "foo"},
	}
	require.NoError(t, writer.WriteSpan(context.Background(), span))
	assert.Eventually(t,
		func() bool {
			pwd, ok := authReceived.Load(upwd1)
			return ok && pwd == upwd1
		},
		5*time.Second, time.Millisecond,
		"expecting es.Client to send the first password",
	)

	t.Log("replace password in the file")
	client1 := getClient()
	newPwdFile := filepath.Join(t.TempDir(), "pwd2")
	require.NoError(t, os.WriteFile(newPwdFile, []byte(pwd2), 0o600))
	require.NoError(t, os.Rename(newPwdFile, pwdFile))

	assert.Eventually(t,
		func() bool {
			client2 := getClient()
			return client1 != client2
		},
		5*time.Second, time.Millisecond,
		"expecting es.Client to change for the new password",
	)

	require.NoError(t, writer.WriteSpan(context.Background(), span))
	assert.Eventually(t,
		func() bool {
			pwd, ok := authReceived.Load(upwd2)
			return ok && pwd == upwd2
		},
		5*time.Second, time.Millisecond,
		"expecting es.Client to send the new password",
	)
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

func getTestingFactoryBase(t *testing.T) *FactoryBase {
	f := &FactoryBase{}
	err := SetFactoryForTest(f, zaptest.NewLogger(t), metrics.NullFactory, &escfg.Configuration{})
	require.NoError(t, err)
	return f
}

func getTestingFactoryBaseWithCreateTemplateError(t *testing.T, cfg *escfg.Configuration, templateErr error) *FactoryBase {
	f := &FactoryBase{}
	err := SetFactoryForTestWithCreateTemplateErr(f, zaptest.NewLogger(t), metrics.NullFactory, cfg, templateErr)
	require.NoError(t, err)
	return f
}
