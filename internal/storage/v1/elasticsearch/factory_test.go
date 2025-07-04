// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/internal/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore"
	esDepStorev2 "github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/depstore"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

var mockEsServerResponse = []byte(`
{
	"Version": {
		"Number": "6"
	}
}
`)

func TestElasticsearchFactoryBase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(mockEsServerResponse)
	}))
	t.Cleanup(server.Close)
	cfg := escfg.Configuration{
		Servers:  []string{server.URL},
		LogLevel: "debug",
		Authentication: escfg.Authentication{
			BasicAuthentication: configoptional.Some(escfg.BasicAuthentication{}),
		},
	}
	f, err := NewFactoryBase(context.Background(), cfg, metrics.NullFactory, zaptest.NewLogger(t))
	require.NoError(t, err)
	readerParams := f.GetSpanReaderParams()
	assert.IsType(t, spanstore.SpanReaderParams{}, readerParams)
	writerParams := f.GetSpanWriterParams()
	assert.IsType(t, spanstore.SpanWriterParams{}, writerParams)
	depParams := f.GetDependencyStoreParams()
	assert.IsType(t, esDepStorev2.Params{}, depParams)
	_, err = f.CreateSamplingStore(1)
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

func TestFactoryBase_Purge(t *testing.T) {
	tests := []struct {
		name        string
		setupMock   func(*mocks.IndicesDeleteService)
		expectedErr bool
	}{
		{
			name: "successful purge",
			setupMock: func(mockDelete *mocks.IndicesDeleteService) {
				mockDelete.On("Do", mock.Anything).Return(nil, nil)
			},
			expectedErr: false,
		},
		{
			name: "purge error",
			setupMock: func(mockDelete *mocks.IndicesDeleteService) {
				mockDelete.On("Do", mock.Anything).Return(nil, errors.New("delete error"))
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a real factory with a mock ES client
			mockClient := &mocks.Client{}
			mockDelete := &mocks.IndicesDeleteService{}
			mockClient.On("DeleteIndex", "*").Return(mockDelete)

			tt.setupMock(mockDelete)

			// Create a mock client that will be stored in the atomic.Pointer
			f := &FactoryBase{
				client: atomic.Pointer[es.Client]{},
			}
			// Create a concrete type that implements es.Client
			var client es.Client = mockClient
			// Store the client in the atomic.Pointer
			f.client.Store(&client)

			err := f.Purge(context.Background())
			if tt.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			// Verify the mock was called as expected
			mockClient.AssertExpectations(t)
			mockDelete.AssertExpectations(t)
		})
	}
}

func TestElasticsearchTagsFileDoNotExist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(mockEsServerResponse)
	}))
	t.Cleanup(server.Close)
	cfg := escfg.Configuration{
		Servers: []string{server.URL},
		Tags: escfg.TagsAsFields{
			File: "fixtures/file-does-not-exist.txt",
		},
		LogLevel: "debug",
	}
	f, err := NewFactoryBase(context.Background(), cfg, metrics.NullFactory, zaptest.NewLogger(t))
	require.ErrorContains(t, err, "open fixtures/file-does-not-exist.txt: no such file or directory")
	assert.Nil(t, f)
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
		f.newClientFn = func(_ context.Context, _ *escfg.Configuration, _ *zap.Logger, _ metrics.Factory) (es.Client, error) {
			return mockClient, nil
		}
		f.logger = zaptest.NewLogger(t)
		f.metricsFactory = metrics.NullFactory
		f.config = &escfg.Configuration{CreateIndexTemplates: true, Indices: escfg.Indices{
			IndexPrefix: test.indexPrefix,
			Spans: escfg.IndexOptions{
				Shards:   3,
				Replicas: ptr(int64(1)),
				Priority: 10,
			},
			Services: escfg.IndexOptions{
				Shards:   3,
				Replicas: ptr(int64(1)),
				Priority: 10,
			},
		}}
		f.tracer = otel.GetTracerProvider()
		client, err := f.newClientFn(context.Background(), &escfg.Configuration{}, zaptest.NewLogger(t), metrics.NullFactory)
		require.NoError(t, err)
		f.client.Store(&client)
		f.templateBuilder = es.TextTemplateBuilder{}
		jaegerSpanId := test.indexPrefix.Apply("jaeger-span")
		jaegerServiceId := test.indexPrefix.Apply("jaeger-service")
		mockClient.On("CreateTemplate", jaegerSpanId).Return(test.spanTemplateService())
		mockClient.On("CreateTemplate", jaegerServiceId).Return(test.serviceTemplateService())
		err = f.createTemplates(context.Background())
		if test.err != "" {
			require.ErrorContains(t, err, test.err)
		} else {
			require.NoError(t, err)
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

func TestPasswordFromFile(t *testing.T) {
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	t.Run("primary client", func(t *testing.T) {
		testPasswordFromFile(t)
	})

	t.Run("load token error", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "does not exist")
		token, err := loadTokenFromFile(file)
		require.Error(t, err)
		assert.Empty(t, token)
	})
}

func testPasswordFromFile(t *testing.T) {
	const (
		pwd1 = "first password"
		pwd2 = "second password"
		// and with user name
		upwd1 = "user:" + pwd1
		upwd2 = "user:" + pwd2
	)
	var authReceived sync.Map
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
	t.Cleanup(server.Close)

	pwdFile := filepath.Join(t.TempDir(), "pwd")
	require.NoError(t, os.WriteFile(pwdFile, []byte(pwd1), 0o600))

	cfg := escfg.Configuration{
		Servers:  []string{server.URL},
		LogLevel: "debug",
		Authentication: escfg.Authentication{
			BasicAuthentication: configoptional.Some(escfg.BasicAuthentication{
				Username:         "user",
				PasswordFilePath: pwdFile,
			}),
		},
		BulkProcessing: escfg.BulkProcessing{
			MaxBytes: -1, // disable bulk; we want immediate flush
		},
	}
	f, err := NewFactoryBase(context.Background(), cfg, metrics.NullFactory, zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, f.Close())
	})

	writer := spanstore.NewSpanWriter(f.GetSpanWriterParams())
	span1 := &dbmodel.Span{
		Process: dbmodel.Process{ServiceName: "foo"},
	}
	writer.WriteSpan(time.Now(), span1)
	assert.Eventually(t,
		func() bool {
			pwd, ok := authReceived.Load(upwd1)
			return ok && pwd == upwd1
		},
		5*time.Second, time.Millisecond,
		"expecting es.Client to send the first password",
	)

	t.Log("replace password in the file")
	client1 := f.getClient()
	newPwdFile := filepath.Join(t.TempDir(), "pwd2")
	require.NoError(t, os.WriteFile(newPwdFile, []byte(pwd2), 0o600))
	require.NoError(t, os.Rename(newPwdFile, pwdFile))

	assert.Eventually(t,
		func() bool {
			client2 := f.getClient()
			return client1 != client2
		},
		5*time.Second, time.Millisecond,
		"expecting es.Client to change for the new password",
	)

	span2 := &dbmodel.Span{
		Process: dbmodel.Process{ServiceName: "foo"},
	}
	writer.WriteSpan(time.Now(), span2)
	assert.Eventually(t,
		func() bool {
			pwd, ok := authReceived.Load(upwd2)
			return ok && pwd == upwd2
		},
		5*time.Second, time.Millisecond,
		"expecting es.Client to send the new password",
	)
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
			BasicAuthentication: configoptional.Some(escfg.BasicAuthentication{
				PasswordFilePath: pwdFile,
			}),
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

func TestFactoryBase_OnClientPasswordChange(t *testing.T) {
	tests := []struct {
		name           string
		setupConfig    func() *escfg.Configuration
		expectedLogs   []string
		shouldCallLoad bool
	}{
		{
			name: "basic authentication not configured",
			setupConfig: func() *escfg.Configuration {
				return &escfg.Configuration{
					Servers: []string{"http://localhost:9200"},
					// Authentication is not set up at all
				}
			},
			expectedLogs:   []string{"basic authentication not configured"},
			shouldCallLoad: false,
		},
		{
			name: "password file path not set",
			setupConfig: func() *escfg.Configuration {
				return &escfg.Configuration{
					Servers: []string{"http://localhost:9200"},
					Authentication: escfg.Authentication{
						BasicAuthentication: configoptional.Some(escfg.BasicAuthentication{
							// No PasswordFilePath set
							Username: "user",
							Password: "pass",
						}),
					},
				}
			},
			expectedLogs:   []string{"password file path not set"},
			shouldCallLoad: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup logger that captures logs
			logger, logs := setupTestLogger()

			// Create a test factory with the test configuration
			cfg := tt.setupConfig()

			fb := &FactoryBase{
				logger:         logger,
				metricsFactory: metrics.NullFactory,
				config:         cfg,
				// Mock the newClientFn to fail the test if it's called unexpectedly
				newClientFn: func(_ context.Context, _ *escfg.Configuration, _ *zap.Logger, _ metrics.Factory) (es.Client, error) {
					t.Fatal("newClientFn should not be called in this test case")
					return nil, nil
				},
			}

			// Create a dummy client to pass to onClientPasswordChange
			var client atomic.Pointer[es.Client]

			// Execute the function under test
			fb.onClientPasswordChange(cfg, &client, metrics.NullFactory)

			// Verify logs
			loggedMsgs := getLoggedMessages(logs)
			for _, expectedLog := range tt.expectedLogs {
				assert.Contains(t, loggedMsgs, expectedLog, "Expected log message not found")
			}

			// Verify client was not modified
			nilClient := (*es.Client)(nil)
			assert.Equal(t, nilClient, client.Load(), "Client should not be modified in error cases")
		})
	}
}

// setupTestLogger creates a logger that captures logs for testing
func setupTestLogger() (*zap.Logger, *zaptest.Buffer) {
	buffer := &zaptest.Buffer{}
	logger := zap.New(
		zapcore.NewCore(
			zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
			buffer,
			zap.InfoLevel,
		),
	)
	return logger, buffer
}

// getLoggedMessages extracts log messages from the buffer and parses them as JSON
func getLoggedMessages(buffer *zaptest.Buffer) []string {
	var messages []string
	for _, line := range buffer.Lines() {
		// Parse the JSON log entry
		var entry struct {
			Msg string `json:"msg"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err == nil {
			messages = append(messages, entry.Msg)
		}
	}
	return messages
}
