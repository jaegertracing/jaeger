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
	esSpanStore "github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore"
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
	f := NewFactoryV1()
	f.coreFactory = getTestingFactoryBase(t)
	v, command := config.Viperize(f.AddFlags)
	command.ParseFlags([]string{})
	f.InitFromViper(v, zap.NewNop())
	require.NoError(t, f.Initialize(metrics.NullFactory, zaptest.NewLogger(t)))
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
			logger := zap.NewNop()
			metricsFactory := metrics.NullFactory
			w := esSpanStore.NewSpanWriterV1(esSpanStore.SpanWriterParams{
				Client: func() es.Client {
					clFxn := (&mockClientBuilder{createTemplateError: errors.New("template-error")}).NewClient
					client, err := clFxn(tt.cfg, logger, metricsFactory)
					require.NoError(t, err)
					return client
				},
				Logger:         logger,
				MetricsFactory: metricsFactory,
			})
			err := createTemplates(w, tt.cfg)
			if tt.expectErr {
				require.ErrorContains(t, err, "template-error")
			} else {
				require.NoError(t, err)
			}
		})
	}
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
	primaryFactory := NewFactoryV1()
	primaryFactory.coreFactory = getTestingFactoryBase(t)
	primaryFactory.coreFactory.SetConfig(primaryConfig)
	archiveConfig := &escfg.Configuration{
		SendGetBodyAs: "PUT",
	}
	archiveFactory := NewFactoryV1()
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
			coreFactory := NewArchiveFactoryBase()
			coreFactory.newClientFn = (&mockClientBuilder{}).NewClient
			f := FactoryV1{coreFactory: coreFactory, Options: coreFactory.Options}
			v, command := config.Viperize(f.AddFlags)
			command.ParseFlags(test.args)
			f.InitFromViper(v, zap.NewNop())

			require.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))

			require.Equal(t, test.expectedReadAlias, f.coreFactory.GetConfig().ReadAliasSuffix)
			require.Equal(t, test.expectedWriteAlias, f.coreFactory.GetConfig().WriteAliasSuffix)
			require.True(t, f.coreFactory.GetConfig().UseReadWriteAliases)
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

func TestConfigureFromOptions(t *testing.T) {
	f := NewFactoryV1()
	o := &Options{
		Config: namespaceConfig{Configuration: escfg.Configuration{Servers: []string{"server"}}},
	}
	f.configureFromOptions(o)
	assert.Equal(t, o.GetConfig(), f.coreFactory.GetConfig())
}

func getTestingFactory(t *testing.T, pwdFile string, server *httptest.Server) *FactoryV1 {
	require.NoError(t, os.WriteFile(pwdFile, []byte(pwd1), 0o600))
	cfg := &escfg.Configuration{
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
	coreFactory := NewFactoryBase()
	coreFactory.SetConfig(cfg)
	f := NewArchiveFactoryV1()
	f.coreFactory = coreFactory
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
