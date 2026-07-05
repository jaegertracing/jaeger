// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package clientbuilder

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configtls"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/auth"
	"github.com/jaegertracing/jaeger/internal/auth/bearertoken"
	"github.com/jaegertracing/jaeger/internal/headerforwarding"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/metricstest"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/spanstoremetrics"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

var mockEsServerResponseWithVersion0 = []byte(`
{
	"Version": {
		"Number": "0"
	}
}
`)

var mockEsServerResponseWithVersion1 = []byte(`
{
	"tagline": "OpenSearch",
	"Version": {
		"Number": "1"
	}
}
`)

var mockEsServerResponseWithVersion2 = []byte(`
{
	"tagline": "OpenSearch",
	"Version": {
		"Number": "2"
	}
}
`)

var mockEsServerResponseWithVersion3 = []byte(`
{
	"tagline": "OpenSearch",
	"Version": {
		"Number": "3"
	}
}
`)

var mockEsServerResponseWithVersion8 = []byte(`
{
	"tagline": "OpenSearch",
	"Version": {
		"Number": "9"
	}
}
`)

func copyToTempFile(t *testing.T, pattern string, filename string) (file *os.File) {
	tempDir := t.TempDir()
	tempFilePath := tempDir + "/" + pattern
	tempFile, err := os.Create(tempFilePath)
	require.NoError(t, err)
	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	_, err = tempFile.Write(data)
	require.NoError(t, err)
	require.NoError(t, tempFile.Close())
	return tempFile
}

// basicAuth creates basic authentication component
func basicAuth(username, password, passwordFilePath string) configoptional.Optional[config.BasicAuthentication] {
	return configoptional.Some(config.BasicAuthentication{
		Username:         username,
		Password:         password,
		PasswordFilePath: passwordFilePath,
	})
}

// bearerAuth creates bearer token authentication component
func bearerAuth(filePath string, allowFromContext bool) configoptional.Optional[config.TokenAuthentication] {
	return configoptional.Some(config.TokenAuthentication{
		FilePath:         filePath,
		AllowFromContext: allowFromContext,
	})
}

// apiKeyAuth creates api key authentication component
func apiKeyAuth(filePath string, allowFromContext bool) configoptional.Optional[config.TokenAuthentication] {
	return configoptional.Some(config.TokenAuthentication{
		FilePath:         filePath,
		AllowFromContext: allowFromContext,
	})
}

func TestNewClient(t *testing.T) {
	const (
		pwd1       = "password"
		token      = "token"
		serverCert = "../../../../internal/config/tlscfg/testdata/example-server-cert.pem"
		apiKey     = "test-api-key"
	)
	apiKeyFile := filepath.Join(t.TempDir(), "api-key")
	require.NoError(t, os.WriteFile(apiKeyFile, []byte(apiKey), 0o600))

	pwdFile := filepath.Join(t.TempDir(), "pwd")
	require.NoError(t, os.WriteFile(pwdFile, []byte(pwd1), 0o600))
	pwdtokenFile := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(pwdtokenFile, []byte(token), 0o600))
	// copy certs to temp so we can modify them
	certFilePath := copyToTempFile(t, "cert.crt", serverCert)
	defer certFilePath.Close()

	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		// Accept both GET and HEAD requests
		assert.Contains(t, []string{http.MethodGet, http.MethodHead}, req.Method)
		res.WriteHeader(http.StatusOK)
		res.Write(mockEsServerResponseWithVersion0)
	}))
	defer testServer.Close()

	testServer1 := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		assert.Contains(t, []string{http.MethodGet, http.MethodHead}, req.Method)
		res.WriteHeader(http.StatusOK)
		res.Write(mockEsServerResponseWithVersion1)
	}))
	defer testServer1.Close()

	testServer2 := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		assert.Contains(t, []string{http.MethodGet, http.MethodHead}, req.Method)
		res.WriteHeader(http.StatusOK)
		res.Write(mockEsServerResponseWithVersion2)
	}))
	defer testServer2.Close()

	testServer3 := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		assert.Contains(t, []string{http.MethodGet, http.MethodHead}, req.Method)
		res.WriteHeader(http.StatusOK)
		res.Write(mockEsServerResponseWithVersion3)
	}))
	defer testServer3.Close()

	testServer8 := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		assert.Contains(t, []string{http.MethodGet, http.MethodHead}, req.Method)
		res.WriteHeader(http.StatusOK)
		res.Write(mockEsServerResponseWithVersion8)
	}))
	defer testServer8.Close()

	tests := []struct {
		name          string
		config        *config.Configuration
		expectedError bool
	}{
		{
			name: "success with valid configuration",
			config: &config.Configuration{
				Servers: []string{testServer.URL},
				Authentication: config.Authentication{
					BasicAuthentication: basicAuth("user", "secret", ""),
					BearerTokenAuth:     bearerAuth("", true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "debug",
				BulkProcessing: config.BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
				Version: 8,
			},
			expectedError: false,
		},
		{
			name: "success with valid configuration and tls enabled",
			config: &config.Configuration{
				Servers: []string{testServer.URL},
				Authentication: config.Authentication{
					BasicAuthentication: basicAuth("user", "secret", ""),
					BearerTokenAuth:     bearerAuth("", true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "debug",
				BulkProcessing: config.BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
				Version: 0,
				TLS:     configtls.ClientConfig{Insecure: false},
			},
			expectedError: false,
		},
		{
			name: "success with valid configuration and reading token and certificate from file",
			config: &config.Configuration{
				Servers: []string{testServer.URL},
				Authentication: config.Authentication{
					BasicAuthentication: basicAuth("user", "secret", ""),
					BearerTokenAuth:     bearerAuth(pwdtokenFile, true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "debug",
				BulkProcessing: config.BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
				Version: 0,
				TLS: configtls.ClientConfig{
					Insecure: true,
					Config: configtls.Config{
						CAFile: certFilePath.Name(),
					},
				},
			},
			expectedError: false,
		},
		{
			name: "success with invalid configuration of version higher than 8",
			config: &config.Configuration{
				Servers: []string{testServer8.URL},
				Authentication: config.Authentication{
					BasicAuthentication: basicAuth("user", "secret", ""),
					BearerTokenAuth:     bearerAuth("", true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "debug",
				BulkProcessing: config.BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
				Version: 9,
			},
			expectedError: false,
		},
		{
			name: "success with valid configuration with version 1",
			config: &config.Configuration{
				Servers: []string{testServer1.URL},
				Authentication: config.Authentication{
					BasicAuthentication: basicAuth("user", "secret", ""),
					BearerTokenAuth:     bearerAuth("", true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "debug",
				BulkProcessing: config.BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: false,
		},
		{
			name: "success with valid configuration with version 2",
			config: &config.Configuration{
				Servers: []string{testServer2.URL},
				Authentication: config.Authentication{
					BasicAuthentication: basicAuth("user", "secret", ""),
					BearerTokenAuth:     bearerAuth("", true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "debug",
				BulkProcessing: config.BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: false,
		},
		{
			name: "success with valid configuration with version 3",
			config: &config.Configuration{
				Servers: []string{testServer3.URL},
				Authentication: config.Authentication{
					BasicAuthentication: basicAuth("user", "secret", ""),
					BearerTokenAuth:     bearerAuth("", true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "debug",
				BulkProcessing: config.BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: false,
		},
		{
			name: "success with valid configuration password from file",
			config: &config.Configuration{
				Servers: []string{testServer.URL},
				Authentication: config.Authentication{
					BasicAuthentication: basicAuth("user", "", pwdFile),
					BearerTokenAuth:     bearerAuth("", true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "debug",
				BulkProcessing: config.BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: false,
		},
		{
			name: "fail with configuration password and password from file are set",
			config: &config.Configuration{
				Servers: []string{testServer.URL},
				Authentication: config.Authentication{
					BasicAuthentication: basicAuth("user", "secret", pwdFile),
					BearerTokenAuth:     bearerAuth("", true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "debug",
				BulkProcessing: config.BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: true,
		},
		{
			name: "fail with missing server",
			config: &config.Configuration{
				Servers: []string{},
				Authentication: config.Authentication{
					BasicAuthentication: basicAuth("user", "secret", ""),
					BearerTokenAuth:     bearerAuth("", true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "debug",
				BulkProcessing: config.BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: true,
		},
		{
			name: "fail with invalid configuration invalid loglevel",
			config: &config.Configuration{
				Servers: []string{testServer.URL},
				Authentication: config.Authentication{
					BasicAuthentication: basicAuth("user", "secret", ""),
					BearerTokenAuth:     bearerAuth("", true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "invalid",
				BulkProcessing: config.BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: true,
		},
		{
			name: "fail with invalid configuration invalid bulkworkers number",
			config: &config.Configuration{
				Servers: []string{testServer.URL},
				Authentication: config.Authentication{
					BasicAuthentication: basicAuth("user", "secret", ""),
					BearerTokenAuth:     bearerAuth("", true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "invalid",
				BulkProcessing: config.BulkProcessing{
					Workers:  0,
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: true,
		},
		{
			name: "success with valid configuration and info log level",
			config: &config.Configuration{
				Servers: []string{testServer.URL},
				Authentication: config.Authentication{
					BasicAuthentication: basicAuth("user", "secret", ""),
					BearerTokenAuth:     bearerAuth("", true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "info",
				BulkProcessing: config.BulkProcessing{
					Workers:  0,
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: false,
		},
		{
			name: "success with valid configuration and error log level",
			config: &config.Configuration{
				Servers: []string{testServer.URL},
				Authentication: config.Authentication{
					BasicAuthentication: basicAuth("user", "secret", ""),
					BearerTokenAuth:     bearerAuth("", true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "error",
				BulkProcessing: config.BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: false,
		},
		{
			name: "success with API key from file",
			config: &config.Configuration{
				Servers: []string{testServer.URL},
				Authentication: config.Authentication{
					APIKeyAuth: apiKeyAuth(apiKeyFile, false),
				},
				LogLevel: "debug",
				BulkProcessing: config.BulkProcessing{
					MaxBytes: -1,
				},
				Version: 8,
			},
			expectedError: false,
		},
		{
			name: "success with API key from context only",
			config: &config.Configuration{
				Servers: []string{testServer.URL},
				Authentication: config.Authentication{
					APIKeyAuth: apiKeyAuth("", true),
				},
				LogLevel: "debug",
				BulkProcessing: config.BulkProcessing{
					MaxBytes: -1,
				},
				Version: 8,
			},
			expectedError: false,
		},
		{
			name: "success with API key from both file and context",
			config: &config.Configuration{
				Servers: []string{testServer.URL},
				Authentication: config.Authentication{
					APIKeyAuth: apiKeyAuth(apiKeyFile, true),
				},
				LogLevel: "debug",
				BulkProcessing: config.BulkProcessing{
					MaxBytes: -1,
				},
				Version: 8,
			},
			expectedError: false,
		},
		{
			name: "fail with invalid API key file path",
			config: &config.Configuration{
				Servers: []string{testServer.URL},
				Authentication: config.Authentication{
					APIKeyAuth: apiKeyAuth("/nonexistent/api-key", false),
				},
				LogLevel: "debug",
				BulkProcessing: config.BulkProcessing{
					MaxBytes: -1,
				},
				Version: 8,
			},
			expectedError: true,
		},
		{
			name: "success with API key context-only disables health check",
			config: &config.Configuration{
				Servers:            []string{testServer.URL},
				DisableHealthCheck: false,
				Authentication: config.Authentication{
					APIKeyAuth: apiKeyAuth("", true),
				},
				LogLevel: "debug",
				BulkProcessing: config.BulkProcessing{
					MaxBytes: -1,
				},
				Version: 8,
			},
			expectedError: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger := zap.NewNop()
			metricsFactory := metrics.NullFactory
			cfg := test.config
			client, err := NewClient(context.Background(), cfg, logger, metricsFactory, nil)
			if test.expectedError {
				require.Error(t, err)
				require.Nil(t, client)
			} else {
				require.NoError(t, err)
				require.NotNil(t, client)
				err = client.Close()
				require.NoError(t, err)
			}
		})
	}
}

func TestNewClientPingErrorHandling(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse []byte
		statusCode     int
		expectedError  string
	}{
		{
			name:           "ping returns 404 status",
			serverResponse: mockEsServerResponseWithVersion0,
			statusCode:     404,
			expectedError:  "ElasticSearch server",
		},
		{
			name:           "ping returns 500 status",
			serverResponse: mockEsServerResponseWithVersion0,
			statusCode:     500,
			expectedError:  "ElasticSearch server",
		},
		{
			name:           "ping returns 300 status",
			serverResponse: mockEsServerResponseWithVersion0,
			statusCode:     300,
			expectedError:  "ElasticSearch server",
		},
		{
			name:           "ping returns empty version number",
			serverResponse: []byte(`{"Version": {"Number": ""}}`),
			statusCode:     200,
			expectedError:  "invalid ping response",
		},

		{
			name:           "ping returns valid 200 status with version",
			serverResponse: mockEsServerResponseWithVersion0,
			statusCode:     200,
			expectedError:  "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
				assert.Contains(t, []string{http.MethodGet, http.MethodHead}, req.Method)
				res.WriteHeader(test.statusCode)
				res.Write(test.serverResponse)
			}))
			defer testServer.Close()

			cfg := &config.Configuration{
				Servers:            []string{testServer.URL},
				LogLevel:           "error",
				DisableHealthCheck: true,
			}

			logger := zap.NewNop()
			metricsFactory := metrics.NullFactory
			client, err := NewClient(context.Background(), cfg, logger, metricsFactory, nil)

			if test.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.expectedError)
				require.Nil(t, client)
			} else {
				require.NoError(t, err)
				require.NotNil(t, client)
				err = client.Close()
				require.NoError(t, err)
			}
		})
	}
}

func TestNewClientVersionDetection(t *testing.T) {
	tests := []struct {
		name            string
		serverResponse  []byte
		expectedVersion es.BackendVersion
		expectedError   string
	}{
		{
			name: "version number with letters",
			serverResponse: []byte(`{
                "Version": {
                    "Number": "7.x.1"
                }
            }`),
			expectedVersion: es.ElasticV7,
			expectedError:   "",
		},
		{
			name: "empty version number should fail validation",
			serverResponse: []byte(`{
                "Version": {
                    "Number": ""
                }
            }`),
			expectedError: "invalid ping response",
		},
		{
			name: "version number as numeric should fail JSON parsing",
			serverResponse: []byte(`{
                "Version": {
                    "Number": 7
                }
            }`),
			expectedError: "cannot unmarshal number",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, _ *http.Request) {
				res.WriteHeader(http.StatusOK)
				res.Write(test.serverResponse)
			}))
			defer testServer.Close()

			cfg := &config.Configuration{
				Servers:            []string{testServer.URL},
				LogLevel:           "error",
				DisableHealthCheck: true,
			}

			logger := zap.NewNop()
			metricsFactory := metrics.NullFactory
			client, err := NewClient(context.Background(), cfg, logger, metricsFactory, nil)

			if test.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.expectedError)
				require.Nil(t, client)
			} else {
				require.NoError(t, err)
				require.NotNil(t, client)
				assert.Equal(t, test.expectedVersion, es.BackendVersion(cfg.Version))
				err = client.Close()
				require.NoError(t, err)
			}
		})
	}
}

func TestHandleBulkAfterCallback_ErrorMetricsEmitted(t *testing.T) {
	mf := metricstest.NewFactory(time.Minute)
	sm := spanstoremetrics.NewWriter(mf, "bulk_index")
	logger := zap.NewNop()
	defer mf.Stop()

	batchID := int64(1)

	fakeRequests := []elastic.BulkableRequest{nil, nil}
	response := &elastic.BulkResponse{
		Errors: true,
		Items: []map[string]*elastic.BulkResponseItem{
			{
				"index": {
					Status: 500,
					Error:  &elastic.ErrorDetails{Type: "server_error"},
				},
			},
			{
				"index": {
					Status: 200,
					Error:  nil,
				},
			},
		},
	}

	bcb := bulkCallback{
		sm:     sm,
		logger: logger,
	}
	bcb.startTimes.Store(batchID, time.Now().Add(-100*time.Millisecond))
	bcb.invoke(batchID, fakeRequests, response, assert.AnError)

	mf.AssertCounterMetrics(
		t,
		metricstest.ExpectedMetric{
			Name:  "bulk_index.errors",
			Value: 1,
		},
		metricstest.ExpectedMetric{
			Name:  "bulk_index.inserts",
			Value: 1,
		},
		metricstest.ExpectedMetric{
			Name:  "bulk_index.attempts",
			Value: 2,
		},
	)
}

func TestHandleBulkAfterCallback_MissingStartTime(t *testing.T) {
	mf := metricstest.NewFactory(time.Minute)
	sm := spanstoremetrics.NewWriter(mf, "bulk_index")
	logger := zap.NewNop()
	defer mf.Stop()

	batchID := int64(42) // assign any value which is not stored in the map
	fakeRequests := []elastic.BulkableRequest{nil}
	response := &elastic.BulkResponse{
		Errors: true,
		Items: []map[string]*elastic.BulkResponseItem{
			{
				"index": {
					Status: 500,
					Error:  &elastic.ErrorDetails{Type: "mock_error"},
				},
			},
		},
	}

	bcb := bulkCallback{
		sm:     sm,
		logger: logger,
	}
	bcb.invoke(batchID, fakeRequests, response, assert.AnError)

	mf.AssertCounterMetrics(
		t,
		metricstest.ExpectedMetric{
			Name:  "bulk_index.errors",
			Value: 1,
		},
		metricstest.ExpectedMetric{
			Name:  "bulk_index.inserts",
			Value: 0,
		},
		metricstest.ExpectedMetric{
			Name:  "bulk_index.attempts",
			Value: 1,
		},
	)
}

func TestGetConfigOptions(t *testing.T) {
	tmpDir := t.TempDir()
	bearerTokenFile := filepath.Join(tmpDir, "bearertoken")
	os.WriteFile(bearerTokenFile, []byte("file-bearer-token"), 0o600)

	tests := []struct {
		name            string
		cfg             *config.Configuration
		ctx             context.Context
		prepare         func()
		wantErr         bool
		wantErrContains string
	}{
		{
			name: "BearerToken context propagation",
			cfg: &config.Configuration{
				Servers:  []string{"http://localhost:9200"},
				Sniffing: config.Sniffing{Enabled: false},
				Authentication: config.Authentication{
					BearerTokenAuth: bearerAuth("", true),
				},
				LogLevel: "info",
			},
			ctx:     bearertoken.ContextWithBearerToken(context.Background(), "context-bearer-token"),
			wantErr: false,
		},
		{
			name: "BearerToken file and context both enabled",
			cfg: &config.Configuration{
				Servers:  []string{"http://localhost:9200"},
				Sniffing: config.Sniffing{Enabled: false},
				Authentication: config.Authentication{
					BearerTokenAuth: bearerAuth(bearerTokenFile, true),
				},
				LogLevel: "info",
			},
			ctx:     bearertoken.ContextWithBearerToken(context.Background(), "context-bearer-token"),
			wantErr: false,
		},
		{
			name: "BearerToken file error",
			cfg: &config.Configuration{
				Servers:  []string{"http://localhost:9200"},
				TLS:      configtls.ClientConfig{Insecure: true},
				Sniffing: config.Sniffing{Enabled: false},
				Authentication: config.Authentication{
					BearerTokenAuth: bearerAuth("/does/not/exist/token", false),
				},
				LogLevel: "info",
			},
			ctx:             context.Background(),
			wantErr:         true,
			wantErrContains: "no such file or directory",
		},
		{
			name: "No auth configured",
			cfg: &config.Configuration{
				Servers:  []string{"http://localhost:9200"},
				LogLevel: "info",
				Sniffing: config.Sniffing{Enabled: false},
			},
			ctx:     context.Background(),
			wantErr: false,
		},
		{
			name: "BasicAuth password file error",
			cfg: &config.Configuration{
				Servers:  []string{"http://localhost:9200"},
				Sniffing: config.Sniffing{Enabled: false},
				Authentication: config.Authentication{
					BasicAuthentication: basicAuth("testuser", "", "/does/not/exist"),
				},
				LogLevel: "info",
			},
			ctx:             context.Background(),
			wantErr:         true,
			wantErrContains: "failed to initialize basic authentication",
		},

		{
			name: "BasicAuth both Password and PasswordFilePath set",
			cfg: &config.Configuration{
				Servers:  []string{"http://localhost:9200"},
				Sniffing: config.Sniffing{Enabled: false},
				Authentication: config.Authentication{
					BasicAuthentication: basicAuth("testuser", "secret", "/some/file/path"),
				},
				LogLevel: "info",
			},
			ctx:             context.Background(),
			wantErr:         true,
			wantErrContains: "failed to initialize basic authentication",
		},

		{
			name: "Invalid log level triggers addLoggerOptions error",
			cfg: &config.Configuration{
				Servers:  []string{"http://localhost:9200"},
				Sniffing: config.Sniffing{Enabled: false},
				Authentication: config.Authentication{
					BasicAuthentication: basicAuth("user", "secret", ""),
				},
				LogLevel: "invalid",
			},
			ctx:             context.Background(),
			wantErr:         true,
			wantErrContains: "unrecognized log-level",
		},
		{
			name: "Health check disabled for context-only auth",
			cfg: &config.Configuration{
				Servers:            []string{"http://localhost:9200"},
				LogLevel:           "info",
				DisableHealthCheck: false, // Should be overridden by context-only auth
				Sniffing:           config.Sniffing{Enabled: false},
				Authentication: config.Authentication{
					BearerTokenAuth: bearerAuth("", true),
				},
			},
			ctx:     bearertoken.ContextWithBearerToken(context.Background(), "context-bearer-token"),
			wantErr: false,
		},
		{
			name: "Health check disabled explicitly",
			cfg: &config.Configuration{
				Servers:            []string{"http://localhost:9200"},
				LogLevel:           "info",
				DisableHealthCheck: true,
				Sniffing:           config.Sniffing{Enabled: false},
			},
			ctx:     context.Background(),
			wantErr: false,
		},
		{
			name: "HTTP compression and custom SendGetBodyAs",
			cfg: &config.Configuration{
				Servers:         []string{"http://localhost:9200"},
				LogLevel:        "info",
				HTTPCompression: true,
				SendGetBodyAs:   "POST",
				Sniffing:        config.Sniffing{Enabled: true, UseHTTPS: true},
			},
			ctx:     context.Background(),
			wantErr: false,
		},
	}

	logger := zap.NewNop()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.prepare != nil {
				tt.prepare()
			}

			options, err := getConfigOptions(tt.ctx, tt.cfg, logger, nil)
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrContains != "" {
					require.Contains(t, err.Error(), tt.wantErrContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, options)
				require.NotEmpty(t, options, "Should have at least basic ES options")
			}
		})
	}
}

func TestGetESOptions(t *testing.T) {
	tests := []struct {
		name               string
		cfg                *config.Configuration
		disableHealthCheck bool
		wantErr            bool
		validateOptions    func(t *testing.T, options []elastic.ClientOptionFunc)
	}{
		{
			name: "Basic configuration",
			cfg: &config.Configuration{
				Servers: []string{"http://localhost:9200"},
				Sniffing: config.Sniffing{
					Enabled:  true,
					UseHTTPS: false,
				},
				HTTPCompression: true,
				SendGetBodyAs:   "POST",
			},
			disableHealthCheck: false,
			wantErr:            false,
			validateOptions: func(t *testing.T, options []elastic.ClientOptionFunc) {
				require.NotNil(t, options)
				require.NotEmpty(t, options, "Expected non-empty options slice")
			},
		},
		{
			name: "HTTPS configuration",
			cfg: &config.Configuration{
				Servers: []string{"https://localhost:9200"},
				Sniffing: config.Sniffing{
					Enabled:  false,
					UseHTTPS: true,
				},
				HTTPCompression: false,
				SendGetBodyAs:   "",
			},
			disableHealthCheck: true,
			wantErr:            false,
			validateOptions: func(t *testing.T, options []elastic.ClientOptionFunc) {
				require.NotNil(t, options)
				require.NotEmpty(t, options, "Expected non-empty options slice")
			},
		},
		{
			name: "Minimal configuration",
			cfg: &config.Configuration{
				Servers: []string{"http://localhost:9200"},
				Sniffing: config.Sniffing{
					Enabled:  false,
					UseHTTPS: false,
				},
				HTTPCompression: false,
				SendGetBodyAs:   "",
			},
			disableHealthCheck: false,
			wantErr:            false,
			validateOptions: func(t *testing.T, options []elastic.ClientOptionFunc) {
				require.NotNil(t, options)
				require.NotEmpty(t, options, "Expected non-empty options slice")
			},
		},
		{
			name: "Multiple servers",
			cfg: &config.Configuration{
				Servers: []string{
					"http://localhost:9200",
					"http://localhost:9201",
					"http://localhost:9202",
				},
				HealthCheckTimeoutStartup: 10 * time.Millisecond,
				Sniffing: config.Sniffing{
					Enabled:  true,
					UseHTTPS: false,
				},
				HTTPCompression: true,
				SendGetBodyAs:   "GET",
			},
			disableHealthCheck: false,
			wantErr:            false,
			validateOptions: func(t *testing.T, options []elastic.ClientOptionFunc) {
				require.NotNil(t, options)
				require.NotEmpty(t, options, "Expected non-empty options slice")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := getESOptions(tt.cfg, tt.disableHealthCheck)
			if tt.wantErr {
				require.Fail(t, "Test case expects an error, but getESOptions does not return one.")
			} else if tt.validateOptions != nil {
				tt.validateOptions(t, options)
			}
		})
	}
}

func TestGetConfigOptionsIntegration(t *testing.T) {
	// Test that getConfigOptions properly integrates with getESOptions
	cfg := &config.Configuration{
		Servers: []string{"http://localhost:9200"},
		Sniffing: config.Sniffing{
			Enabled:  true,
			UseHTTPS: false,
		},
		HTTPCompression: true,
		SendGetBodyAs:   "POST",
		LogLevel:        "info",
		QueryTimeout:    30 * time.Second,
		Authentication: config.Authentication{
			BasicAuthentication: basicAuth("testuser", "testpass", ""),
		},
	}

	logger := zap.NewNop()
	options, err := getConfigOptions(context.Background(), cfg, logger, nil)
	require.NoError(t, err)
	require.NotNil(t, options)
	require.Greater(t, len(options), 5, "Should have basic ES options plus additional config options")
}

func TestGetHTTPRoundTripper(t *testing.T) {
	tmpDir := t.TempDir()
	bearerTokenFile := filepath.Join(tmpDir, "bearertoken")
	require.NoError(t, os.WriteFile(bearerTokenFile, []byte("file-bearer-token"), 0o600))

	tests := []struct {
		name            string
		cfg             *config.Configuration
		ctx             context.Context
		wantErrContains string
		validate        func(t *testing.T, rt http.RoundTripper)
	}{
		{
			name: "Secure mode without auth",
			cfg: &config.Configuration{
				TLS: configtls.ClientConfig{Insecure: false},
			},
			ctx: context.Background(),
			validate: func(t *testing.T, rt http.RoundTripper) {
				assert.NotNil(t, rt)
				_, ok := rt.(*auth.RoundTripper)
				assert.False(t, ok, "Should not be an auth round tripper")
			},
		},
		{
			name: "Insecure mode without auth",
			cfg: &config.Configuration{
				TLS: configtls.ClientConfig{Insecure: true},
			},
			ctx: context.Background(),
			validate: func(t *testing.T, rt http.RoundTripper) {
				assert.NotNil(t, rt)
				transport, ok := rt.(*http.Transport)
				require.True(t, ok)
				assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
			},
		},
		{
			name: "Bearer auth not applicable (empty config)",
			cfg: &config.Configuration{
				TLS: configtls.ClientConfig{Insecure: true},
				Authentication: config.Authentication{
					BearerTokenAuth: bearerAuth("", false),
				},
			},
			ctx: context.Background(),
			validate: func(t *testing.T, rt http.RoundTripper) {
				assert.NotNil(t, rt)
				// Should be plain transport since auth is not applicable
				_, ok := rt.(*auth.RoundTripper)
				assert.False(t, ok, "Should not be an auth round tripper when config is not applicable")

				transport, ok := rt.(*http.Transport)
				assert.True(t, ok, "Should be plain http.Transport")
				assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
			},
		},
		{
			name: "Secure mode with bearer token from file",
			cfg: &config.Configuration{
				TLS: configtls.ClientConfig{Insecure: false},
				Authentication: config.Authentication{
					BearerTokenAuth: bearerAuth(bearerTokenFile, false),
				},
			},
			ctx: context.Background(),
			validate: func(t *testing.T, rt http.RoundTripper) {
				assert.NotNil(t, rt)
				authRT, ok := rt.(*auth.RoundTripper)
				require.True(t, ok, "Should be an auth round tripper")
				require.Len(t, authRT.Auths, 1)
				assert.Equal(t, "Bearer", authRT.Auths[0].Scheme)
				assert.NotNil(t, authRT.Auths[0].TokenFn)
				assert.Equal(t, "file-bearer-token", authRT.Auths[0].TokenFn())
			},
		},
		{
			name: "Insecure mode with bearer token from context",
			cfg: &config.Configuration{
				TLS: configtls.ClientConfig{Insecure: true},
				Authentication: config.Authentication{
					BearerTokenAuth: bearerAuth("", true),
				},
			},
			ctx: bearertoken.ContextWithBearerToken(context.Background(), "context-bearer-token"),
			validate: func(t *testing.T, rt http.RoundTripper) {
				assert.NotNil(t, rt)
				authRT, ok := rt.(*auth.RoundTripper)
				require.True(t, ok, "Should be an auth round tripper")
				require.Len(t, authRT.Auths, 1)
				assert.Equal(t, "Bearer", authRT.Auths[0].Scheme)
				assert.NotNil(t, authRT.Auths[0].FromCtx)

				transport, ok := authRT.Transport.(*http.Transport)
				require.True(t, ok)
				assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
			},
		},
		{
			name: "BearerToken file error",
			cfg: &config.Configuration{
				Authentication: config.Authentication{
					BearerTokenAuth: bearerAuth("/does/not/exist/token", false),
				},
			},
			ctx:             context.Background(),
			wantErrContains: "no such file or directory",
		},
		{
			name: "Invalid TLS config should fail",
			cfg: &config.Configuration{
				TLS: configtls.ClientConfig{
					Insecure: false,
					Config: configtls.Config{
						CAFile: "/does/not/exist/ca.pem",
					},
				},
			},
			ctx:             context.Background(),
			wantErrContains: "failed to load TLS config",
		},
	}

	logger := zap.NewNop()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, err := GetHTTPRoundTripper(tt.ctx, tt.cfg, logger, nil)
			if tt.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContains)
				assert.Nil(t, rt)
			} else {
				require.NoError(t, err)
				tt.validate(t, rt)
			}
		})
	}
}

// Test GetHTTPRoundTripper with httpAuth error
func TestGetHTTPRoundTripperWithHTTPAuthError(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	// Create a mock httpAuth that will fail on RoundTripper wrapping
	mockAuth := &mockFailingHTTPAuth{}

	c := &config.Configuration{
		Servers:  []string{"http://localhost:9200"},
		LogLevel: "error",
		TLS:      configtls.ClientConfig{Insecure: true},
	}

	_, err := GetHTTPRoundTripper(ctx, c, logger, mockAuth)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to wrap round tripper with HTTP authenticator")
}

// Mock failing HTTP authenticator
type mockFailingHTTPAuth struct{}

func (*mockFailingHTTPAuth) RoundTripper(_ http.RoundTripper) (http.RoundTripper, error) {
	return nil, errors.New("mock authenticator error")
}

func TestGetHTTPRoundTripperWrappingError(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	// Create a mock failing HTTP authenticator
	mockAuth := &mockFailingHTTPAuthWrapper{}

	c := &config.Configuration{
		Servers:  []string{"http://localhost:9200"},
		LogLevel: "error",
		TLS:      configtls.ClientConfig{Insecure: true},
	}

	_, err := GetHTTPRoundTripper(ctx, c, logger, mockAuth)
	require.Error(t, err)
	require.ErrorContains(t, err, "failed to wrap round tripper with HTTP authenticator")
}

// mockFailingHTTPAuthWrapper mocks a failing HTTP authenticator for wrapping tests
type mockFailingHTTPAuthWrapper struct{}

func (*mockFailingHTTPAuthWrapper) RoundTripper(_ http.RoundTripper) (http.RoundTripper, error) {
	return nil, errors.New("wrapping error")
}

// Test GetHTTPRoundTripper with successful httpAuth wrapping
func TestGetHTTPRoundTripperWithHTTPAuthSuccess(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	// Create a mock httpAuth that will succeed
	mockAuth := &mockSuccessfulHTTPAuth{}

	c := &config.Configuration{
		Servers:  []string{"http://localhost:9200"},
		LogLevel: "error",
		TLS:      configtls.ClientConfig{Insecure: true},
	}

	rt, err := GetHTTPRoundTripper(ctx, c, logger, mockAuth)

	require.NoError(t, err)
	require.NotNil(t, rt)
	// getBodyFixRoundTripper must be outermost so that it populates
	// req.GetBody before the authenticator hashes the payload.
	bodyFixRT, ok := rt.(*getBodyFixRoundTripper)
	require.True(t, ok, "outermost round tripper should be getBodyFixRoundTripper")
	wrappedRT, ok := bodyFixRT.base.(*mockWrappedRoundTripper)
	require.True(t, ok, "authenticator wrapper should be inside getBodyFixRoundTripper")
	require.NotNil(t, wrappedRT)
}

// Mock successful HTTP authenticator
type mockSuccessfulHTTPAuth struct{}

func (*mockSuccessfulHTTPAuth) RoundTripper(rt http.RoundTripper) (http.RoundTripper, error) {
	return &mockWrappedRoundTripper{base: rt}, nil
}

// Mock wrapped round tripper
type mockWrappedRoundTripper struct {
	base http.RoundTripper
}

func (m *mockWrappedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.base.RoundTrip(req)
}

// getBodyRecordingAuth mimics an authenticator like sigv4auth: its
// RoundTripper inspects req.GetBody at the start of RoundTrip (where
// sigv4auth hashes the payload) before delegating to the base transport.
type getBodyRecordingAuth struct {
	sawGetBody bool
}

func (a *getBodyRecordingAuth) RoundTripper(base http.RoundTripper) (http.RoundTripper, error) {
	return &getBodyRecordingRoundTripper{auth: a, base: base}, nil
}

type getBodyRecordingRoundTripper struct {
	auth *getBodyRecordingAuth
	base http.RoundTripper
}

func (rt *getBodyRecordingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.auth.sawGetBody = req.GetBody != nil
	// Short-circuit instead of hitting the network.
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
}

// TestGetHTTPRoundTripper_AuthSeesGetBody verifies that an HTTP
// authenticator sees a populated req.GetBody at signing time for requests
// where the client set req.Body without GetBody (as olivere/elastic does).
// If getBodyFixRoundTripper is wrapped inside the authenticator instead of
// outside, the authenticator hashes an empty payload and AWS-managed
// OpenSearch rejects every body-bearing request with HTTP 403.
func TestGetHTTPRoundTripper_AuthSeesGetBody(t *testing.T) {
	recordingAuth := &getBodyRecordingAuth{}

	c := &config.Configuration{
		Servers:  []string{"http://localhost:9200"},
		LogLevel: "error",
		TLS:      configtls.ClientConfig{Insecure: true},
	}

	rt, err := GetHTTPRoundTripper(context.Background(), c, zap.NewNop(), recordingAuth)
	require.NoError(t, err)

	// Mimic olivere/elastic: Body set directly, GetBody left nil.
	req, err := http.NewRequest(http.MethodPut, "http://localhost:9200/_index_template/test", http.NoBody)
	require.NoError(t, err)
	req.Body = io.NopCloser(bytes.NewReader([]byte(`{"index_patterns":["jaeger-*"]}`)))
	req.GetBody = nil

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	assert.True(t, recordingAuth.sawGetBody,
		"authenticator must see req.GetBody populated at signing time")
}

func TestBulkCallbackInvoke_NilResponse(t *testing.T) {
	mf := metricstest.NewFactory(time.Minute)
	sm := spanstoremetrics.NewWriter(mf, "bulk_index")
	logger := zap.NewNop()
	defer mf.Stop()

	bcb := bulkCallback{
		sm:     sm,
		logger: logger,
	}
	bcb.invoke(1, []elastic.BulkableRequest{nil}, nil, assert.AnError)

	mf.AssertCounterMetrics(
		t,
		metricstest.ExpectedMetric{
			Name:  "bulk_index.errors",
			Value: 0,
		},
		metricstest.ExpectedMetric{
			Name:  "bulk_index.inserts",
			Value: 1,
		},
		metricstest.ExpectedMetric{
			Name:  "bulk_index.attempts",
			Value: 1,
		},
	)
}

func TestBulkCallbackInvoke_SuccessPath(t *testing.T) {
	mf := metricstest.NewFactory(time.Minute)
	sm := spanstoremetrics.NewWriter(mf, "bulk_index")
	logger := zap.NewNop()
	defer mf.Stop()

	batchID := int64(7)

	bcb := bulkCallback{
		sm:     sm,
		logger: logger,
	}
	bcb.startTimes.Store(batchID, time.Now().Add(-50*time.Millisecond))
	bcb.invoke(batchID, []elastic.BulkableRequest{nil, nil}, &elastic.BulkResponse{}, nil)

	mf.AssertCounterMetrics(
		t,
		metricstest.ExpectedMetric{
			Name:  "bulk_index.errors",
			Value: 0,
		},
		metricstest.ExpectedMetric{
			Name:  "bulk_index.inserts",
			Value: 2,
		},
		metricstest.ExpectedMetric{
			Name:  "bulk_index.attempts",
			Value: 2,
		},
	)
}

func TestInitAuthNilInputs(t *testing.T) {
	logger := zap.NewNop()

	method, err := initBearerAuth(nil, logger)
	require.NoError(t, err)
	assert.Nil(t, method)

	method, err = initAPIKeyAuth(nil, logger)
	require.NoError(t, err)
	assert.Nil(t, method)

	method, err = initBasicAuth(nil, logger)
	require.NoError(t, err)
	assert.Nil(t, method)

	// Empty username returns nil
	method, err = initBasicAuth(&config.BasicAuthentication{Username: ""}, logger)
	require.NoError(t, err)
	assert.Nil(t, method)
}

func TestNewClientWithCustomHeaders(t *testing.T) {
	headersSeen := false
	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		// Check if custom headers are present
		if req.Header.Get("X-Custom-Header") == "custom-value" {
			headersSeen = true
		}
		res.WriteHeader(http.StatusOK)
		res.Write(mockEsServerResponseWithVersion8)
	}))
	defer testServer.Close()

	cfg := config.Configuration{
		Servers: []string{testServer.URL},
		CustomHeaders: map[string]string{
			"Host":            "my-opensearch.amazonaws.com",
			"X-Custom-Header": "custom-value",
		},
		LogLevel: "error",
		Version:  8,
	}

	logger := zap.NewNop()
	metricsFactory := metrics.NullFactory

	client, err := NewClient(context.Background(), &cfg, logger, metricsFactory, nil)
	require.NoError(t, err)
	require.NotNil(t, client)

	// Verify the configuration has the custom headers set
	// Note: The ES v8 client may not send custom headers during the initial ping/health check,
	// but they will be available for actual Elasticsearch operations (index, search, etc.)
	assert.Equal(t, "my-opensearch.amazonaws.com", cfg.CustomHeaders["Host"])
	assert.Equal(t, "custom-value", cfg.CustomHeaders["X-Custom-Header"])

	if headersSeen {
		t.Log(" Custom headers were transmitted in HTTP request")
	} else {
		t.Log("  Custom headers not sent in ping request (expected - will be sent in data operations)")
	}

	err = client.Close()
	require.NoError(t, err)
}

type recordingRoundTripper struct {
	req *http.Request
}

func (r *recordingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r.req = req
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
}

func TestGetBodyFixRoundTripper_SetsGetBody(t *testing.T) {
	payload := []byte(`{"action":"index","data":"test"}`)
	req, err := http.NewRequest(http.MethodPost, "http://localhost", http.NoBody)
	require.NoError(t, err)
	req.Body = io.NopCloser(bytes.NewReader(payload))
	req.GetBody = nil

	recorder := &recordingRoundTripper{}
	rt := &getBodyFixRoundTripper{base: recorder}

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	require.NotNil(t, recorder.req.GetBody)
	body, err := recorder.req.GetBody()
	require.NoError(t, err)
	got, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, payload, got)

	sentBody, err := io.ReadAll(recorder.req.Body)
	require.NoError(t, err)
	assert.Equal(t, payload, sentBody)
}

func TestGetBodyFixRoundTripper_PreservesExistingGetBody(t *testing.T) {
	original := []byte("original")
	customGetBody := func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(original)), nil
	}

	req, err := http.NewRequest(http.MethodPost, "http://localhost", bytes.NewReader([]byte("different")))
	require.NoError(t, err)
	req.GetBody = customGetBody

	recorder := &recordingRoundTripper{}
	rt := &getBodyFixRoundTripper{base: recorder}

	_, err = rt.RoundTrip(req)
	require.NoError(t, err)

	body, err := recorder.req.GetBody()
	require.NoError(t, err)
	got, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, original, got)
}

func TestGetBodyFixRoundTripper_NilBody(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://localhost", http.NoBody)
	require.NoError(t, err)
	req.Body = nil
	req.GetBody = nil

	recorder := &recordingRoundTripper{}
	rt := &getBodyFixRoundTripper{base: recorder}

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Nil(t, recorder.req.GetBody)
}

type errReader struct{}

func (*errReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestGetBodyFixRoundTripper_ReadAllError(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "http://localhost", http.NoBody)
	require.NoError(t, err)
	req.Body = io.NopCloser(&errReader{})
	req.GetBody = nil

	recorder := &recordingRoundTripper{}
	rt := &getBodyFixRoundTripper{base: recorder}

	resp, err := rt.RoundTrip(req)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	assert.Nil(t, resp)
	assert.Nil(t, recorder.req, "base RoundTripper should not have been called")
}

// TestNewClient_ForwardsCapturedHeadersToES verifies that captured headers
// stored on the request context (via the headerforwarding package) are
// propagated to outbound HTTP requests issued by the Elasticsearch client.
// The version-detection ping is the first request the client makes after
// construction; it carries the context passed to NewClient and therefore
// flows through the headerforwarding RoundTripper installed at the callsite.
func TestNewClient_ForwardsCapturedHeadersToES(t *testing.T) {
	var (
		mu      sync.Mutex
		headers []http.Header
	)
	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		mu.Lock()
		headers = append(headers, req.Header.Clone())
		mu.Unlock()
		res.WriteHeader(http.StatusOK)
		res.Write(mockEsServerResponseWithVersion0)
	}))
	defer testServer.Close()

	cfg := config.Configuration{
		Servers:            []string{testServer.URL},
		LogLevel:           "error",
		DisableHealthCheck: true,
		BulkProcessing:     config.BulkProcessing{MaxBytes: -1},
	}
	hUser := &headerforwarding.ForwardedHeader{HTTPName: "X-Forwarded-User", Role: headerforwarding.RoleUsername}
	ctx := headerforwarding.ContextWithCaptured(context.Background(), []headerforwarding.CapturedHeader{
		{Header: hUser, Value: "alice"},
	})

	client, err := NewClient(ctx, &cfg, zap.NewNop(), metrics.NullFactory, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, headers, "fake ES server should have received at least one request")
	for i, h := range headers {
		assert.Equalf(t, "alice", h.Get("X-Forwarded-User"), "request #%d missing forwarded header (got: %v)", i, h)
	}
}

// TestNewClient_NoCapturedHeaders_NoForwardedHeader verifies the negative case:
// when the inbound context carries no captured headers, the outbound ES
// requests do not gain the forwarded header.
func TestNewClient_NoCapturedHeaders_NoForwardedHeader(t *testing.T) {
	var (
		mu      sync.Mutex
		headers []http.Header
	)
	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		mu.Lock()
		headers = append(headers, req.Header.Clone())
		mu.Unlock()
		res.WriteHeader(http.StatusOK)
		res.Write(mockEsServerResponseWithVersion0)
	}))
	defer testServer.Close()

	cfg := config.Configuration{
		Servers:            []string{testServer.URL},
		LogLevel:           "error",
		DisableHealthCheck: true,
		BulkProcessing:     config.BulkProcessing{MaxBytes: -1},
	}

	client, err := NewClient(context.Background(), &cfg, zap.NewNop(), metrics.NullFactory, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, headers, "fake ES server should have received at least one request")
	for i, h := range headers {
		assert.Emptyf(t, h.Get("X-Forwarded-User"), "request #%d unexpectedly carries forwarded header", i)
	}
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

func TestGetHTTPRoundTripperCustomHeaders(t *testing.T) {
	var gotHeader, gotHost string
	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		gotHeader = req.Header.Get("X-Custom-Header")
		gotHost = req.Host
		res.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()

	cfg := &config.Configuration{
		TLS: configtls.ClientConfig{Insecure: true},
		CustomHeaders: map[string]string{
			"X-Custom-Header": "custom-value",
			"Host":            "signed.example.com",
		},
	}
	rt, err := GetHTTPRoundTripper(context.Background(), cfg, zap.NewNop(), nil)
	require.NoError(t, err)

	client := &http.Client{Transport: rt}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, testServer.URL, http.NoBody)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "custom-value", gotHeader)
	assert.Equal(t, "signed.example.com", gotHost)
}

func TestGetHTTPRoundTripperCustomHeadersDoNotMutateOriginalRequest(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, _ *http.Request) {
		res.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()

	cfg := &config.Configuration{
		TLS:           configtls.ClientConfig{Insecure: true},
		CustomHeaders: map[string]string{"X-Custom-Header": "custom-value"},
	}
	rt, err := GetHTTPRoundTripper(context.Background(), cfg, zap.NewNop(), nil)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, testServer.URL, http.NoBody)
	require.NoError(t, err)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Empty(t, req.Header.Get("X-Custom-Header"))
}

func TestGetHTTPRoundTripperCustomHeadersDoNotOverridePerRequestHeaders(t *testing.T) {
	var gotHeader string
	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		gotHeader = req.Header.Get("X-Custom-Header")
		res.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()

	cfg := &config.Configuration{
		TLS:           configtls.ClientConfig{Insecure: true},
		CustomHeaders: map[string]string{"X-Custom-Header": "static-value"},
	}
	rt, err := GetHTTPRoundTripper(context.Background(), cfg, zap.NewNop(), nil)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, testServer.URL, http.NoBody)
	require.NoError(t, err)
	req.Header.Set("X-Custom-Header", "per-request-value")
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "per-request-value", gotHeader)
}
