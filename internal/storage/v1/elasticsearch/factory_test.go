// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/extension/extensionauth"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/internal/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/mocks"
	esdepstorev2 "github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
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
	}
	f, err := NewFactoryBase(context.Background(), cfg, metrics.NullFactory, zaptest.NewLogger(t), nil)
	require.NoError(t, err)
	readerParams := f.GetSpanReaderParams()
	assert.IsType(t, core.SpanReaderParams{}, readerParams)
	writerParams := f.GetSpanWriterParams()
	assert.IsType(t, core.SpanWriterParams{}, writerParams)
	depParams := f.GetDependencyStoreParams()
	assert.IsType(t, esdepstorev2.Params{}, depParams)
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
	f, err := NewFactoryBase(context.Background(), cfg, metrics.NullFactory, zaptest.NewLogger(t), nil)
	require.ErrorContains(t, err, "open fixtures/file-does-not-exist.txt: no such file or directory")
	assert.Nil(t, f)
}

func TestTagKeysAsFields(t *testing.T) {
	dir := t.TempDir()

	tagsFile := filepath.Join(dir, "tags.txt")
	require.NoError(t, os.WriteFile(tagsFile, []byte("foo\nbar\n      space   \n"), 0o600))

	emptyFile := filepath.Join(dir, "empty.txt")
	require.NoError(t, os.WriteFile(emptyFile, []byte(""), 0o600))

	missingFile := filepath.Join(dir, "missing.txt") // intentionally not created

	tests := []struct {
		path          string
		include       string
		expected      []string
		errorExpected bool
	}{
		{
			path:          missingFile,
			errorExpected: true,
		},
		{
			path:     tagsFile,
			expected: []string{"foo", "bar", "space"},
		},
		{
			path:     emptyFile,
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
			path:     tagsFile,
			include:  "televators,eriatarka,thewidow",
			expected: []string{"foo", "bar", "space", "televators", "eriatarka", "thewidow"},
		},
		{
			path:     emptyFile,
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
		f.newClientFn = func(_ context.Context, _ *escfg.Configuration, _ *zap.Logger, _ metrics.Factory, _ extensionauth.HTTPClient) (es.Client, error) {
			return mockClient, nil
		}
		f.logger = zaptest.NewLogger(t)
		f.metricsFactory = metrics.NullFactory
		f.config = &escfg.Configuration{CreateIndexTemplates: true, Indices: escfg.Indices{
			IndexPrefix: test.indexPrefix,
			Spans: escfg.IndexOptions{
				Shards:   3,
				Replicas: new(int64(1)),
				Priority: 10,
			},
			Services: escfg.IndexOptions{
				Shards:   3,
				Replicas: new(int64(1)),
				Priority: 10,
			},
		}}
		f.tracer = otel.GetTracerProvider()
		client, err := f.newClientFn(context.Background(), &escfg.Configuration{}, zaptest.NewLogger(t), metrics.NullFactory, nil)
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
	factory, err := NewFactoryBase(context.Background(), cfg, metrics.NullFactory, zap.NewNop(), nil)
	require.NoError(t, err)
	factory.Close()
}

func TestESStorageFactoryWithConfigError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}))
	defer server.Close()
	cfg := escfg.Configuration{
		Servers:            []string{server.URL},
		DisableHealthCheck: true,
		LogLevel:           "error",
	}
	_, err := NewFactoryBase(context.Background(), cfg, metrics.NullFactory, zap.NewNop(), nil)
	require.ErrorContains(t, err, "failed to create Elasticsearch client")
}

func TestPasswordFromFile(t *testing.T) {
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	t.Run("primary client", func(t *testing.T) {
		runPasswordFromFileTest(t)
	})

	t.Run("load token error", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "does not exist")
		token, err := loadTokenFromFile(file)
		require.Error(t, err)
		assert.Empty(t, token)
	})
}

func runPasswordFromFileTest(t *testing.T) {
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
			MaxBytes:   -1, // disable bulk
			MaxActions: -1, // disable bulk; the test only validates auth headers
		},
	}
	f, err := NewFactoryBase(context.Background(), cfg, metrics.NullFactory, zap.NewNop(), nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, f.Close())
	})

	writer := core.NewSpanWriter(f.GetSpanWriterParams())
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
		BulkProcessing: escfg.BulkProcessing{
			MaxBytes:   -1, // disable bulk
			MaxActions: -1, // disable bulk; the test only validates error paths
		},
	}

	logger, buf := testutils.NewEchoLogger(t)
	f, err := NewFactoryBase(context.Background(), cfg, metrics.NullFactory, logger, nil)
	require.NoError(t, err)
	defer f.Close()

	f.config.Servers = []string{}
	f.onPasswordChange()
	assert.Contains(t, buf.String(), "no servers specified")

	require.NoError(t, os.Remove(pwdFile))
	f.onPasswordChange()
}

func TestFactoryBase_NewClient_WatcherError(t *testing.T) {
	cfg := escfg.Configuration{
		Servers:  []string{"http://localhost:9200"},
		LogLevel: "debug",
		Authentication: escfg.Authentication{
			BasicAuthentication: configoptional.Some(escfg.BasicAuthentication{
				Username:         "testuser",
				PasswordFilePath: "/nonexistent/path/to/password.txt",
			}),
		},
	}

	_, err := NewFactoryBase(context.Background(), cfg, metrics.NullFactory, zaptest.NewLogger(t), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initialize basic authentication")
	assert.Contains(t, err.Error(), "failed to get token from file")
}

func TestElasticsearchFactoryBaseWithAuthenticator(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(mockEsServerResponse)
	}))
	t.Cleanup(server.Close)

	cfg := escfg.Configuration{
		Servers:  []string{server.URL},
		LogLevel: "debug",
		BulkProcessing: escfg.BulkProcessing{
			MaxBytes:   -1, // disable bulk
			MaxActions: -1, // disable bulk; the test only validates authenticator setup
		},
	}

	// Mock authenticator
	mockAuth := &mockHTTPAuthenticator{}

	f, err := NewFactoryBase(context.Background(), cfg, metrics.NullFactory, zaptest.NewLogger(t), mockAuth)
	require.NoError(t, err)
	require.NotNil(t, f)
	defer require.NoError(t, f.Close())

	// Verify factory is properly initialized with authenticator
	readerParams := f.GetSpanReaderParams()
	assert.IsType(t, core.SpanReaderParams{}, readerParams)
}

// mockHTTPAuthenticator implements extensionauth.HTTPClient for testing
type mockHTTPAuthenticator struct{}

func (*mockHTTPAuthenticator) RoundTripper(base http.RoundTripper) (http.RoundTripper, error) {
	return &mockRoundTripper{base: base}, nil
}

// mockRoundTripper wraps the base RoundTripper
type mockRoundTripper struct {
	base http.RoundTripper
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer mock-token")
	if m.base != nil {
		return m.base.RoundTrip(req)
	}
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
}

func TestVerifySpanMappingSchema(t *testing.T) {
	templateName := "jaeger-test-jaeger-span"

	tests := []struct {
		name          string
		createMapping bool
		mockSetup     func(*mocks.Client, *mocks.IndicesGetIndexTemplateService)
		expectedError string
	}{
		{
			name:          "CreateIndexTemplates is true",
			createMapping: true,
			expectedError: "",
		},
		{
			name:          "GetTemplate error",
			createMapping: false,
			mockSetup: func(c *mocks.Client, s *mocks.IndicesGetIndexTemplateService) {
				c.On("GetTemplate", templateName).Return(s)
				s.On("Do", mock.Anything).Return(nil, errors.New("ES error"))
			},
			expectedError: "ES error",
		},
		{
			name:          "Template not found",
			createMapping: false,
			mockSetup: func(c *mocks.Client, s *mocks.IndicesGetIndexTemplateService) {
				c.On("GetTemplate", templateName).Return(s)
				s.On("Do", mock.Anything).Return(&elastic.IndicesGetIndexTemplateResponse{}, nil)
			},
			expectedError: fmt.Sprintf("template %q not found", templateName),
		},
		{
			name:          "Missing properties (top level and span level)",
			createMapping: false,
			mockSetup: func(c *mocks.Client, s *mocks.IndicesGetIndexTemplateService) {
				c.On("GetTemplate", templateName).Return(s)
				res := &elastic.IndicesGetIndexTemplateResponse{
					IndexTemplates: elastic.IndicesGetIndexTemplatesSlice{
						{
							Name: templateName,
							IndexTemplate: &elastic.IndicesGetIndexTemplate{
								Template: &elastic.IndicesGetIndexTemplateData{
									Mappings: map[string]any{
										"not_properties": map[string]any{},
									},
								},
							},
						},
					},
				}
				s.On("Do", mock.Anything).Return(res, nil)
			},
			expectedError: fmt.Sprintf("template %q is missing 'scopeTag' field", templateName),
		},
		{
			name:          "ES6 style - missing properties inside span",
			createMapping: false,
			mockSetup: func(c *mocks.Client, s *mocks.IndicesGetIndexTemplateService) {
				c.On("GetTemplate", templateName).Return(s)
				res := &elastic.IndicesGetIndexTemplateResponse{
					IndexTemplates: elastic.IndicesGetIndexTemplatesSlice{
						{
							Name: templateName,
							IndexTemplate: &elastic.IndicesGetIndexTemplate{
								Template: &elastic.IndicesGetIndexTemplateData{
									Mappings: map[string]any{
										"span": map[string]any{
											"not_properties": map[string]any{},
										},
									},
								},
							},
						},
					},
				}
				s.On("Do", mock.Anything).Return(res, nil)
			},
			expectedError: fmt.Sprintf("template %q mapping is missing 'properties'", templateName),
		},
		{
			name:          "Missing scopeTag",
			createMapping: false,
			mockSetup: func(c *mocks.Client, s *mocks.IndicesGetIndexTemplateService) {
				c.On("GetTemplate", templateName).Return(s)
				res := &elastic.IndicesGetIndexTemplateResponse{
					IndexTemplates: elastic.IndicesGetIndexTemplatesSlice{
						{
							Name: templateName,
							IndexTemplate: &elastic.IndicesGetIndexTemplate{
								Template: &elastic.IndicesGetIndexTemplateData{
									Mappings: map[string]any{
										"properties": map[string]any{
											"references": map[string]any{
												"properties": map[string]any{
													"tags": map[string]any{},
												},
											},
										},
									},
								},
							},
						},
					},
				}
				s.On("Do", mock.Anything).Return(res, nil)
			},
			expectedError: fmt.Sprintf("template %q is missing 'scopeTag' field", templateName),
		},
		{
			name:          "Missing references",
			createMapping: false,
			mockSetup: func(c *mocks.Client, s *mocks.IndicesGetIndexTemplateService) {
				c.On("GetTemplate", templateName).Return(s)
				res := &elastic.IndicesGetIndexTemplateResponse{
					IndexTemplates: elastic.IndicesGetIndexTemplatesSlice{
						{
							Name: templateName,
							IndexTemplate: &elastic.IndicesGetIndexTemplate{
								Template: &elastic.IndicesGetIndexTemplateData{
									Mappings: map[string]any{
										"properties": map[string]any{
											"scopeTag": map[string]any{},
										},
									},
								},
							},
						},
					},
				}
				s.On("Do", mock.Anything).Return(res, nil)
			},
			expectedError: fmt.Sprintf("template %q is missing 'references.tags'", templateName),
		},
		{
			name:          "References is not a map",
			createMapping: false,
			mockSetup: func(c *mocks.Client, s *mocks.IndicesGetIndexTemplateService) {
				c.On("GetTemplate", templateName).Return(s)
				res := &elastic.IndicesGetIndexTemplateResponse{
					IndexTemplates: elastic.IndicesGetIndexTemplatesSlice{
						{
							Name: templateName,
							IndexTemplate: &elastic.IndicesGetIndexTemplate{
								Template: &elastic.IndicesGetIndexTemplateData{
									Mappings: map[string]any{
										"properties": map[string]any{
											"scopeTag":   map[string]any{},
											"references": "not a map",
										},
									},
								},
							},
						},
					},
				}
				s.On("Do", mock.Anything).Return(res, nil)
			},
			expectedError: fmt.Sprintf("template %q is missing 'references.tags'", templateName),
		},
		{
			name:          "Missing references.properties",
			createMapping: false,
			mockSetup: func(c *mocks.Client, s *mocks.IndicesGetIndexTemplateService) {
				c.On("GetTemplate", templateName).Return(s)
				res := &elastic.IndicesGetIndexTemplateResponse{
					IndexTemplates: elastic.IndicesGetIndexTemplatesSlice{
						{
							Name: templateName,
							IndexTemplate: &elastic.IndicesGetIndexTemplate{
								Template: &elastic.IndicesGetIndexTemplateData{
									Mappings: map[string]any{
										"properties": map[string]any{
											"scopeTag": map[string]any{},
											"references": map[string]any{
												"not_properties": map[string]any{},
											},
										},
									},
								},
							},
						},
					},
				}
				s.On("Do", mock.Anything).Return(res, nil)
			},
			expectedError: fmt.Sprintf("template %q is missing 'references.tags'", templateName),
		},
		{
			name:          "Missing references.tags",
			createMapping: false,
			mockSetup: func(c *mocks.Client, s *mocks.IndicesGetIndexTemplateService) {
				c.On("GetTemplate", templateName).Return(s)
				res := &elastic.IndicesGetIndexTemplateResponse{
					IndexTemplates: elastic.IndicesGetIndexTemplatesSlice{
						{
							Name: templateName,
							IndexTemplate: &elastic.IndicesGetIndexTemplate{
								Template: &elastic.IndicesGetIndexTemplateData{
									Mappings: map[string]any{
										"properties": map[string]any{
											"scopeTag": map[string]any{},
											"references": map[string]any{
												"properties": map[string]any{
													"not_tags": map[string]any{},
												},
											},
										},
									},
								},
							},
						},
					},
				}
				s.On("Do", mock.Anything).Return(res, nil)
			},
			expectedError: fmt.Sprintf("template %q is missing 'references.tags'", templateName),
		},
		{
			name:          "Success ES7 style",
			createMapping: false,
			mockSetup: func(c *mocks.Client, s *mocks.IndicesGetIndexTemplateService) {
				c.On("GetTemplate", templateName).Return(s)
				res := &elastic.IndicesGetIndexTemplateResponse{
					IndexTemplates: elastic.IndicesGetIndexTemplatesSlice{
						{
							Name: templateName,
							IndexTemplate: &elastic.IndicesGetIndexTemplate{
								Template: &elastic.IndicesGetIndexTemplateData{
									Mappings: map[string]any{
										"properties": map[string]any{
											"scopeTag": map[string]any{},
											"references": map[string]any{
												"properties": map[string]any{
													"tags": map[string]any{},
												},
											},
										},
									},
								},
							},
						},
					},
				}
				s.On("Do", mock.Anything).Return(res, nil)
			},
			expectedError: "",
		},
		{
			name:          "Success ES6 style",
			createMapping: false,
			mockSetup: func(c *mocks.Client, s *mocks.IndicesGetIndexTemplateService) {
				c.On("GetTemplate", templateName).Return(s)
				res := &elastic.IndicesGetIndexTemplateResponse{
					IndexTemplates: elastic.IndicesGetIndexTemplatesSlice{
						{
							Name: templateName,
							IndexTemplate: &elastic.IndicesGetIndexTemplate{
								Template: &elastic.IndicesGetIndexTemplateData{
									Mappings: map[string]any{
										"span": map[string]any{
											"properties": map[string]any{
												"scopeTag": map[string]any{},
												"references": map[string]any{
													"properties": map[string]any{
														"tags": map[string]any{},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				}
				s.On("Do", mock.Anything).Return(res, nil)
			},
			expectedError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mocks.Client{}
			mockService := &mocks.IndicesGetIndexTemplateService{}

			if tt.mockSetup != nil {
				tt.mockSetup(mockClient, mockService)
			}

			cfg := &escfg.Configuration{
				CreateIndexTemplates: tt.createMapping,
				Indices: escfg.Indices{
					IndexPrefix: "jaeger-test",
				},
			}
			f := &FactoryBase{
				config: cfg,
			}
			var client es.Client = mockClient
			f.client.Store(&client)

			err := f.verifySpanMappingSchema(context.Background())
			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
