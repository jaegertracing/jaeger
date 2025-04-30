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
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/config"
	"github.com/jaegertracing/jaeger/internal/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/mocks"
	esSpanStore "github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore"
	esDepStorev2 "github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/depstore"
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
	logger := zaptest.NewLogger(t)
	withMockCoreFactory(logger, func(m *mocks.CoreFactory) {
		f := NewFactoryV1()
		f.coreFactory = m
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
	})
}

func withMockCoreFactory(logger *zap.Logger, testingFxn func(m *mocks.CoreFactory)) {
	coreFactory := &mocks.CoreFactory{}
	registerMockCoreFactoryMethods(logger, coreFactory)
	testingFxn(coreFactory)
}

func registerMockCoreFactoryMethods(logger *zap.Logger, coreFactory *mocks.CoreFactory) {
	coreFactory.On("GetSpanReaderParams").Return(esSpanStore.SpanReaderParams{Logger: logger}, nil)
	coreFactory.On("GetSpanWriterParams").Return(esSpanStore.SpanWriterParams{Logger: logger}, nil)
	coreFactory.On("GetDependencyStoreParams").Return(esDepStorev2.Params{Logger: logger})
	coreFactory.On("AddFlags", mock.Anything).Return()
	coreFactory.On("InitFromViper", mock.Anything, mock.Anything).Return()
	coreFactory.On("Close").Return(nil)
	coreFactory.On("GetMetricsFactory").Return(metrics.NullFactory)
	coreFactory.On("GetConfig").Return(&escfg.Configuration{})
	coreFactory.On("CreateSamplingStore", mock.Anything).Return(nil, nil)
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
		coreFactory, ok := f.coreFactory.(*Factory)
		require.True(t, ok)
		testPasswordFromFile(t, pwdFile, authReceived, coreFactory.getClient, f.CreateSpanWriter)
	})
	t.Run("load token error", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "does not exist")
		token, err := loadTokenFromFile(file)
		require.Error(t, err)
		assert.Empty(t, token)
	})
}

func TestInheritSettingsFrom(t *testing.T) {
	logger := zaptest.NewLogger(t)
	primaryConfig := escfg.Configuration{
		MaxDocCount:        99,
		RemoteReadClusters: []string{},
	}
	primaryFactory := NewFactoryV1()
	primaryCoreFactory := &mocks.CoreFactory{}
	registerMockCoreFactoryMethods(logger, primaryCoreFactory)
	primaryCoreFactory.On("GetConfig").Return(&primaryConfig)
	primaryFactory.coreFactory = primaryCoreFactory
	archiveConfig := escfg.Configuration{
		SendGetBodyAs:      "PUT",
		RemoteReadClusters: []string{},
	}
	archiveFactory := NewFactoryV1()
	archiveCoreFactory := &mocks.CoreFactory{}
	registerMockCoreFactoryMethods(logger, archiveCoreFactory)
	archiveCoreFactory.On("GetConfig").Return(&archiveConfig)
	archiveFactory.Options = NewOptions(archiveNamespace)
	archiveFactory.coreFactory = archiveCoreFactory
	archiveFactory.InheritSettingsFrom(primaryFactory)
	require.Equal(t, "PUT", archiveConfig.SendGetBodyAs)
	require.Equal(t, 99, primaryConfig.MaxDocCount)
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
	f, err := NewFactoryV1WithConfig(*cfg, metrics.NullFactory, zaptest.NewLogger(t))
	require.NoError(t, err)
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
