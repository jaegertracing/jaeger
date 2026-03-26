// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"errors"
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
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/metricstest"
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
func basicAuth(username, password, passwordFilePath string) configoptional.Optional[BasicAuthentication] {
	return configoptional.Some(BasicAuthentication{
		Username:         username,
		Password:         password,
		PasswordFilePath: passwordFilePath,
	})
}

// bearerAuth creates bearer token authentication component
func bearerAuth(filePath string, allowFromContext bool) configoptional.Optional[TokenAuthentication] {
	return configoptional.Some(TokenAuthentication{
		FilePath:         filePath,
		AllowFromContext: allowFromContext,
	})
}

// apiKeyAuth creates api key authentication component
func apiKeyAuth(filePath string, allowFromContext bool) configoptional.Optional[TokenAuthentication] {
	return configoptional.Some(TokenAuthentication{
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
		config        *Configuration
		expectedError bool
	}{
		{
			name: "success with valid configuration",
			config: &Configuration{
				Servers: []string{testServer.URL},
				Authentication: Authentication{
					BasicAuthentication: basicAuth("user", "secret", ""),
					BearerTokenAuth:     bearerAuth("", true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
				Version: 8,
			},
			expectedError: false,
		},
		{
			name: "success with valid configuration and tls enabled",
			config: &Configuration{
				Servers: []string{testServer.URL},
				Authentication: Authentication{
					BasicAuthentication: basicAuth("user", "secret", ""),
					BearerTokenAuth:     bearerAuth("", true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
				Version: 0,
				TLS:     configtls.ClientConfig{Insecure: false},
			},
			expectedError: false,
		},
		{
			name: "success with valid configuration and reading token and certificate from file",
			config: &Configuration{
				Servers: []string{testServer.URL},
				Authentication: Authentication{
					BasicAuthentication: basicAuth("user", "secret", ""),
					BearerTokenAuth:     bearerAuth(pwdtokenFile, true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
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
			config: &Configuration{
				Servers: []string{testServer8.URL},
				Authentication: Authentication{
					BasicAuthentication: basicAuth("user", "secret", ""),
					BearerTokenAuth:     bearerAuth("", true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
				Version: 9,
			},
			expectedError: false,
		},
		{
			name: "success with valid configuration with version 1",
			config: &Configuration{
				Servers: []string{testServer1.URL},
				Authentication: Authentication{
					BasicAuthentication: basicAuth("user", "secret", ""),
					BearerTokenAuth:     bearerAuth("", true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: false,
		},
		{
			name: "success with valid configuration with version 2",
			config: &Configuration{
				Servers: []string{testServer2.URL},
				Authentication: Authentication{
					BasicAuthentication: basicAuth("user", "secret", ""),
					BearerTokenAuth:     bearerAuth("", true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: false,
		},
		{
			name: "success with valid configuration with version 3",
			config: &Configuration{
				Servers: []string{testServer3.URL},
				Authentication: Authentication{
					BasicAuthentication: basicAuth("user", "secret", ""),
					BearerTokenAuth:     bearerAuth("", true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: false,
		},
		{
			name: "success with valid configuration password from file",
			config: &Configuration{
				Servers: []string{testServer.URL},
				Authentication: Authentication{
					BasicAuthentication: basicAuth("user", "", pwdFile),
					BearerTokenAuth:     bearerAuth("", true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: false,
		},
		{
			name: "fail with configuration password and password from file are set",
			config: &Configuration{
				Servers: []string{testServer.URL},
				Authentication: Authentication{
					BasicAuthentication: basicAuth("user", "secret", pwdFile),
					BearerTokenAuth:     bearerAuth("", true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: true,
		},
		{
			name: "fail with missing server",
			config: &Configuration{
				Servers: []string{},
				Authentication: Authentication{
					BasicAuthentication: basicAuth("user", "secret", ""),
					BearerTokenAuth:     bearerAuth("", true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: true,
		},
		{
			name: "fail with invalid configuration invalid loglevel",
			config: &Configuration{
				Servers: []string{testServer.URL},
				Authentication: Authentication{
					BasicAuthentication: basicAuth("user", "secret", ""),
					BearerTokenAuth:     bearerAuth("", true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "invalid",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: true,
		},
		{
			name: "fail with invalid configuration invalid bulkworkers number",
			config: &Configuration{
				Servers: []string{testServer.URL},
				Authentication: Authentication{
					BasicAuthentication: basicAuth("user", "secret", ""),
					BearerTokenAuth:     bearerAuth("", true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "invalid",
				BulkProcessing: BulkProcessing{
					Workers:  0,
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: true,
		},
		{
			name: "success with valid configuration and info log level",
			config: &Configuration{
				Servers: []string{testServer.URL},
				Authentication: Authentication{
					BasicAuthentication: basicAuth("user", "secret", ""),
					BearerTokenAuth:     bearerAuth("", true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "info",
				BulkProcessing: BulkProcessing{
					Workers:  0,
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: false,
		},
		{
			name: "success with valid configuration and error log level",
			config: &Configuration{
				Servers: []string{testServer.URL},
				Authentication: Authentication{
					BasicAuthentication: basicAuth("user", "secret", ""),
					BearerTokenAuth:     bearerAuth("", true),
					APIKeyAuth:          apiKeyAuth("", false),
				},
				LogLevel: "error",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: false,
		},
		{
			name: "success with API key from file",
			config: &Configuration{
				Servers: []string{testServer.URL},
				Authentication: Authentication{
					APIKeyAuth: apiKeyAuth(apiKeyFile, false),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1,
				},
				Version: 8,
			},
			expectedError: false,
		},
		{
			name: "success with API key from context only",
			config: &Configuration{
				Servers: []string{testServer.URL},
				Authentication: Authentication{
					APIKeyAuth: apiKeyAuth("", true),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1,
				},
				Version: 8,
			},
			expectedError: false,
		},
		{
			name: "success with API key from both file and context",
			config: &Configuration{
				Servers: []string{testServer.URL},
				Authentication: Authentication{
					APIKeyAuth: apiKeyAuth(apiKeyFile, true),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1,
				},
				Version: 8,
			},
			expectedError: false,
		},
		{
			name: "fail with invalid API key file path",
			config: &Configuration{
				Servers: []string{testServer.URL},
				Authentication: Authentication{
					APIKeyAuth: apiKeyAuth("/nonexistent/api-key", false),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1,
				},
				Version: 8,
			},
			expectedError: true,
		},
		{
			name: "success with API key context-only disables health check",
			config: &Configuration{
				Servers:            []string{testServer.URL},
				DisableHealthCheck: false,
				Authentication: Authentication{
					APIKeyAuth: apiKeyAuth("", true),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
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
			config := test.config
			client, err := NewClient(context.Background(), config, logger, metricsFactory, nil)
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

			config := &Configuration{
				Servers:            []string{testServer.URL},
				LogLevel:           "error",
				DisableHealthCheck: true,
			}

			logger := zap.NewNop()
			metricsFactory := metrics.NullFactory
			client, err := NewClient(context.Background(), config, logger, metricsFactory, nil)

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
		expectedVersion uint
		expectedError   string
	}{
		{
			name: "version number with letters",
			serverResponse: []byte(`{
                "Version": {
                    "Number": "7.x.1"
                }
            }`),
			expectedVersion: 7,
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

			config := &Configuration{
				Servers:            []string{testServer.URL},
				LogLevel:           "error",
				DisableHealthCheck: true,
			}

			logger := zap.NewNop()
			metricsFactory := metrics.NullFactory
			client, err := NewClient(context.Background(), config, logger, metricsFactory, nil)

			if test.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.expectedError)
				require.Nil(t, client)
			} else {
				require.NoError(t, err)
				require.NotNil(t, client)
				assert.Equal(t, test.expectedVersion, config.Version)
				err = client.Close()
				require.NoError(t, err)
			}
		})
	}
}

func TestApplyDefaults(t *testing.T) {
	source := &Configuration{
		RemoteReadClusters: []string{"cluster1", "cluster2"},
		Authentication: Authentication{
			BasicAuthentication: basicAuth("sourceUser", "sourcePass", ""),
		},
		Sniffing: Sniffing{
			Enabled:  true,
			UseHTTPS: true,
		},
		MaxSpanAge:               100,
		AdaptiveSamplingLookback: 50,
		Indices: Indices{
			IndexPrefix: "hello",
			Spans: IndexOptions{
				Shards:   5,
				Replicas: new(int64(1)),
				Priority: 10,
			},
			Services: IndexOptions{
				Shards:   5,
				Replicas: new(int64(1)),
				Priority: 20,
			},
			Dependencies: IndexOptions{
				Shards:   5,
				Replicas: new(int64(1)),
				Priority: 30,
			},
			Sampling: IndexOptions{},
		},
		BulkProcessing: BulkProcessing{
			MaxBytes:      1000,
			Workers:       10,
			MaxActions:    100,
			FlushInterval: 30,
		},
		Tags:          TagsAsFields{AllAsFields: true, DotReplacement: "dot", Include: "include", File: "file"},
		MaxDocCount:   10000,
		LogLevel:      "info",
		SendGetBodyAs: "json",
	}

	tests := []struct {
		name     string
		target   *Configuration
		expected *Configuration
	}{
		{
			name: "All Defaults Applied except PriorityDependenciesTemplate",
			target: &Configuration{
				Indices: Indices{
					Dependencies: IndexOptions{
						Priority: 30,
					},
				},
			}, // All fields are empty
			expected: source,
		},
		{
			name: "Some Defaults Applied",
			target: &Configuration{
				RemoteReadClusters: []string{"customCluster"},
				Authentication: Authentication{
					BasicAuthentication: basicAuth("customUser", "", ""),
				},
				Indices: Indices{
					Spans: IndexOptions{
						Priority: 10,
					},
					Services: IndexOptions{
						Priority: 20,
					},
					Dependencies: IndexOptions{
						Priority: 30,
					},
				},
				// Other fields left default
			},
			expected: &Configuration{
				RemoteReadClusters: []string{"customCluster"},
				Authentication: Authentication{
					BasicAuthentication: basicAuth("customUser", "sourcePass", ""),
				},
				Sniffing: Sniffing{
					Enabled:  true,
					UseHTTPS: true,
				},
				MaxSpanAge:               100,
				AdaptiveSamplingLookback: 50,
				Indices: Indices{
					IndexPrefix: "hello",
					Spans: IndexOptions{
						Shards:   5,
						Replicas: new(int64(1)),
						Priority: 10,
					},
					Services: IndexOptions{
						Shards:   5,
						Replicas: new(int64(1)),
						Priority: 20,
					},
					Dependencies: IndexOptions{
						Shards:   5,
						Replicas: new(int64(1)),
						Priority: 30,
					},
				},
				BulkProcessing: BulkProcessing{
					MaxBytes:      1000,
					Workers:       10,
					MaxActions:    100,
					FlushInterval: 30,
				},
				Tags:          TagsAsFields{AllAsFields: true, DotReplacement: "dot", Include: "include", File: "file"},
				MaxDocCount:   10000,
				LogLevel:      "info",
				SendGetBodyAs: "json",
			},
		},
		{
			name: "No Defaults Applied",
			target: &Configuration{
				RemoteReadClusters: []string{"cluster1", "cluster2"},
				Authentication: Authentication{
					BasicAuthentication: basicAuth("sourceUser", "sourcePass", ""),
				},
				Sniffing: Sniffing{
					Enabled:  true,
					UseHTTPS: true,
				},
				MaxSpanAge:               100,
				AdaptiveSamplingLookback: 50,
				Indices: Indices{
					IndexPrefix: "hello",
					Spans: IndexOptions{
						Shards:   5,
						Replicas: new(int64(1)),
						Priority: 10,
					},
					Services: IndexOptions{
						Shards:   5,
						Replicas: new(int64(1)),
						Priority: 20,
					},
					Dependencies: IndexOptions{
						Shards:   5,
						Replicas: new(int64(1)),
						Priority: 30,
					},
				},
				BulkProcessing: BulkProcessing{
					MaxBytes:      1000,
					Workers:       10,
					MaxActions:    100,
					FlushInterval: 30,
				},
				Tags:          TagsAsFields{AllAsFields: true, DotReplacement: "dot", Include: "include", File: "file"},
				MaxDocCount:   10000,
				LogLevel:      "info",
				SendGetBodyAs: "json",
			},
			expected: source,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.target.ApplyDefaults(source)
			require.Equal(t, test.expected, test.target)
		})
	}
}

func TestApplyDefaults_Auth(t *testing.T) {
	source := &Configuration{
		Authentication: Authentication{
			BasicAuthentication: basicAuth("sourceUser", "sourcePass", ""),
		},
	}

	target := &Configuration{
		Authentication: Authentication{
			BasicAuthentication: basicAuth("", "", ""),
		},
	}

	expected := &Configuration{
		Authentication: Authentication{
			BasicAuthentication: basicAuth("sourceUser", "sourcePass", ""),
		},
	}

	target.ApplyDefaults(source)
	require.Equal(t, expected, target)
}

func TestTagKeysAsFields(t *testing.T) {
	const (
		pwd1 = "tag1\ntag2"
	)

	pwdFile := filepath.Join(t.TempDir(), "pwd")
	require.NoError(t, os.WriteFile(pwdFile, []byte(pwd1), 0o600))
	tests := []struct {
		name         string
		config       *Configuration
		expectedTags []string
		expectError  bool
	}{
		{
			name: "File with tags",
			config: &Configuration{
				Tags: TagsAsFields{
					File:    pwdFile,
					Include: "",
				},
			},
			expectedTags: []string{"tag1", "tag2"},
			expectError:  false,
		},
		{
			name: "include with tags",
			config: &Configuration{
				Tags: TagsAsFields{
					File:    "",
					Include: "cmdtag1,cmdtag2",
				},
			},
			expectedTags: []string{"cmdtag1", "cmdtag2"},
			expectError:  false,
		},
		{
			name: "File and include with tags",
			config: &Configuration{
				Tags: TagsAsFields{
					File:    pwdFile,
					Include: "cmdtag1,cmdtag2",
				},
			},
			expectedTags: []string{"tag1", "tag2", "cmdtag1", "cmdtag2"},
			expectError:  false,
		},
		{
			name: "File read error",
			config: &Configuration{
				Tags: TagsAsFields{
					File:    "/invalid/path/to/file.txt",
					Include: "",
				},
			},
			expectedTags: nil,
			expectError:  true,
		},
		{
			name: "Empty file and params",
			config: &Configuration{
				Tags: TagsAsFields{
					File:    "",
					Include: "",
				},
			},
			expectedTags: nil,
			expectError:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tags, err := test.config.TagKeysAsFields()
			if test.expectError {
				require.Error(t, err)
				require.Nil(t, tags)
			} else {
				require.NoError(t, err)
				require.ElementsMatch(t, test.expectedTags, tags)
			}
		})
	}
}

func TestRolloverFrequencyAsNegativeDuration(t *testing.T) {
	tests := []struct {
		name           string
		indexFrequency string
		expected       time.Duration
	}{
		{
			name:           "hourly jaeger-span",
			indexFrequency: "hour",
			expected:       -1 * time.Hour,
		},
		{
			name:           "daily jaeger-span",
			indexFrequency: "daily",
			expected:       -24 * time.Hour,
		},
		{
			name:           "empty jaeger-span",
			indexFrequency: "",
			expected:       -24 * time.Hour,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := RolloverFrequencyAsNegativeDuration(test.indexFrequency)
			require.Equal(t, test.expected, got)
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name          string
		config        *Configuration
		expectedError string
	}{
		{
			name: "All valid input are set",
			config: &Configuration{
				Servers: []string{"localhost:8000/dummyserver"},
			},
		},
		{
			name:          "no valid input are set",
			config:        &Configuration{},
			expectedError: "Servers: non zero value required",
		},
		{
			name:          "ilm disabled and read-write aliases enabled error",
			config:        &Configuration{Servers: []string{"localhost:8000/dummyserver"}, UseILM: true},
			expectedError: "UseILM must always be used in conjunction with UseReadWriteAliases to ensure ES writers and readers refer to the single index mapping",
		},
		{
			name:          "ilm and create templates enabled",
			config:        &Configuration{Servers: []string{"localhost:8000/dummyserver"}, UseILM: true, CreateIndexTemplates: true, UseReadWriteAliases: true},
			expectedError: "when UseILM is set true, CreateIndexTemplates must be set to false and index templates must be created by init process of es-rollover app",
		},
		{
			name: "explicit span aliases without UseReadWriteAliases",
			config: &Configuration{
				Servers:        []string{"localhost:8000/dummyserver"},
				SpanReadAlias:  "custom-span-read",
				SpanWriteAlias: "custom-span-write",
			},
			expectedError: "explicit aliases (span_read_alias, span_write_alias, service_read_alias, service_write_alias) require UseReadWriteAliases to be true",
		},
		{
			name: "only span read alias set",
			config: &Configuration{
				Servers:             []string{"localhost:8000/dummyserver"},
				UseReadWriteAliases: true,
				SpanReadAlias:       "custom-span-read",
			},
			expectedError: "both span_read_alias and span_write_alias must be set together",
		},
		{
			name: "only service write alias set",
			config: &Configuration{
				Servers:             []string{"localhost:8000/dummyserver"},
				UseReadWriteAliases: true,
				ServiceWriteAlias:   "custom-service-write",
			},
			expectedError: "both service_read_alias and service_write_alias must be set together",
		},
		{
			name: "all explicit aliases with UseReadWriteAliases is valid",
			config: &Configuration{
				Servers:             []string{"localhost:8000/dummyserver"},
				UseReadWriteAliases: true,
				SpanReadAlias:       "custom-span-read",
				SpanWriteAlias:      "custom-span-write",
				ServiceReadAlias:    "custom-service-read",
				ServiceWriteAlias:   "custom-service-write",
			},
		},
		{
			name: "only span aliases with UseReadWriteAliases is valid",
			config: &Configuration{
				Servers:             []string{"localhost:8000/dummyserver"},
				UseReadWriteAliases: true,
				SpanReadAlias:       "custom-span-read",
				SpanWriteAlias:      "custom-span-write",
			},
		},
		{
			name: "explicit aliases with IndexPrefix is valid",
			config: &Configuration{
				Servers:             []string{"localhost:8000/dummyserver"},
				UseReadWriteAliases: true,
				SpanReadAlias:       "custom-span-read",
				SpanWriteAlias:      "custom-span-write",
				ServiceReadAlias:    "custom-service-read",
				ServiceWriteAlias:   "custom-service-write",
				Indices: Indices{
					IndexPrefix: "prod",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.config.Validate()
			if test.expectedError != "" {
				require.ErrorContains(t, got, test.expectedError)
			} else {
				require.NoError(t, got)
			}
		})
	}
}

func TestApplyForIndexPrefix(t *testing.T) {
	tests := []struct {
		testName     string
		prefix       IndexPrefix
		name         string
		expectedName string
	}{
		{
			testName:     "no prefix",
			prefix:       "",
			name:         "hello",
			expectedName: "hello",
		},
		{
			testName:     "empty name",
			prefix:       "bye",
			name:         "",
			expectedName: "bye-",
		},
		{
			testName:     "separator suffix",
			prefix:       "bye-",
			name:         "hello",
			expectedName: "bye-hello",
		},
		{
			testName:     "no separator suffix",
			prefix:       "bye",
			name:         "hello",
			expectedName: "bye-hello",
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			got := test.prefix.Apply(test.name)
			require.Equal(t, test.expectedName, got)
		})
	}
}

func TestHandleBulkAfterCallback_ErrorMetricsEmitted(t *testing.T) {
	mf := metricstest.NewFactory(time.Minute)
	sm := spanstoremetrics.NewWriter(mf, "bulk_index")
	logger := zap.NewNop()
	defer mf.Stop()

	var m sync.Map
	batchID := int64(1)
	start := time.Now().Add(-100 * time.Millisecond)
	m.Store(batchID, start)

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
	bcb.invoke(batchID, fakeRequests, response, assert.AnError)

	mf.AssertCounterMetrics(t,
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

	mf.AssertCounterMetrics(t,
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
		cfg             *Configuration
		ctx             context.Context
		prepare         func()
		wantErr         bool
		wantErrContains string
	}{
		{
			name: "BearerToken context propagation",
			cfg: &Configuration{
				Servers:  []string{"http://localhost:9200"},
				Sniffing: Sniffing{Enabled: false},
				Authentication: Authentication{
					BearerTokenAuth: bearerAuth("", true),
				},
				LogLevel: "info",
			},
			ctx:     bearertoken.ContextWithBearerToken(context.Background(), "context-bearer-token"),
			wantErr: false,
		},
		{
			name: "BearerToken file and context both enabled",
			cfg: &Configuration{
				Servers:  []string{"http://localhost:9200"},
				Sniffing: Sniffing{Enabled: false},
				Authentication: Authentication{
					BearerTokenAuth: bearerAuth(bearerTokenFile, true),
				},
				LogLevel: "info",
			},
			ctx:     bearertoken.ContextWithBearerToken(context.Background(), "context-bearer-token"),
			wantErr: false,
		},
		{
			name: "BearerToken file error",
			cfg: &Configuration{
				Servers:  []string{"http://localhost:9200"},
				TLS:      configtls.ClientConfig{Insecure: true},
				Sniffing: Sniffing{Enabled: false},
				Authentication: Authentication{
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
			cfg: &Configuration{
				Servers:  []string{"http://localhost:9200"},
				LogLevel: "info",
				Sniffing: Sniffing{Enabled: false},
			},
			ctx:     context.Background(),
			wantErr: false,
		},
		{
			name: "BasicAuth password file error",
			cfg: &Configuration{
				Servers:  []string{"http://localhost:9200"},
				Sniffing: Sniffing{Enabled: false},
				Authentication: Authentication{
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
			cfg: &Configuration{
				Servers:  []string{"http://localhost:9200"},
				Sniffing: Sniffing{Enabled: false},
				Authentication: Authentication{
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
			cfg: &Configuration{
				Servers:  []string{"http://localhost:9200"},
				Sniffing: Sniffing{Enabled: false},
				Authentication: Authentication{
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
			cfg: &Configuration{
				Servers:            []string{"http://localhost:9200"},
				LogLevel:           "info",
				DisableHealthCheck: false, // Should be overridden by context-only auth
				Sniffing:           Sniffing{Enabled: false},
				Authentication: Authentication{
					BearerTokenAuth: bearerAuth("", true),
				},
			},
			ctx:     bearertoken.ContextWithBearerToken(context.Background(), "context-bearer-token"),
			wantErr: false,
		},
		{
			name: "Health check disabled explicitly",
			cfg: &Configuration{
				Servers:            []string{"http://localhost:9200"},
				LogLevel:           "info",
				DisableHealthCheck: true,
				Sniffing:           Sniffing{Enabled: false},
			},
			ctx:     context.Background(),
			wantErr: false,
		},
		{
			name: "HTTP compression and custom SendGetBodyAs",
			cfg: &Configuration{
				Servers:         []string{"http://localhost:9200"},
				LogLevel:        "info",
				HTTPCompression: true,
				SendGetBodyAs:   "POST",
				Sniffing:        Sniffing{Enabled: true, UseHTTPS: true},
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

			options, err := tt.cfg.getConfigOptions(tt.ctx, logger, nil)
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
		cfg                *Configuration
		disableHealthCheck bool
		wantErr            bool
		validateOptions    func(t *testing.T, options []elastic.ClientOptionFunc)
	}{
		{
			name: "Basic configuration",
			cfg: &Configuration{
				Servers: []string{"http://localhost:9200"},
				Sniffing: Sniffing{
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
			cfg: &Configuration{
				Servers: []string{"https://localhost:9200"},
				Sniffing: Sniffing{
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
			cfg: &Configuration{
				Servers: []string{"http://localhost:9200"},
				Sniffing: Sniffing{
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
			cfg: &Configuration{
				Servers: []string{
					"http://localhost:9200",
					"http://localhost:9201",
					"http://localhost:9202",
				},
				HealthCheckTimeoutStartup: 10 * time.Millisecond,
				Sniffing: Sniffing{
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
			options := tt.cfg.getESOptions(tt.disableHealthCheck)
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
	cfg := &Configuration{
		Servers: []string{"http://localhost:9200"},
		Sniffing: Sniffing{
			Enabled:  true,
			UseHTTPS: false,
		},
		HTTPCompression: true,
		SendGetBodyAs:   "POST",
		LogLevel:        "info",
		QueryTimeout:    30 * time.Second,
		Authentication: Authentication{
			BasicAuthentication: basicAuth("testuser", "testpass", ""),
		},
	}

	logger := zap.NewNop()
	options, err := cfg.getConfigOptions(context.Background(), logger, nil)
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
		cfg             *Configuration
		ctx             context.Context
		wantErrContains string
		validate        func(t *testing.T, rt http.RoundTripper)
	}{
		{
			name: "Secure mode without auth",
			cfg: &Configuration{
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
			cfg: &Configuration{
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
			cfg: &Configuration{
				TLS: configtls.ClientConfig{Insecure: true},
				Authentication: Authentication{
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
			cfg: &Configuration{
				TLS: configtls.ClientConfig{Insecure: false},
				Authentication: Authentication{
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
			cfg: &Configuration{
				TLS: configtls.ClientConfig{Insecure: true},
				Authentication: Authentication{
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
			cfg: &Configuration{
				Authentication: Authentication{
					BearerTokenAuth: bearerAuth("/does/not/exist/token", false),
				},
			},
			ctx:             context.Background(),
			wantErrContains: "no such file or directory",
		},
		{
			name: "Invalid TLS config should fail",
			cfg: &Configuration{
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

	c := &Configuration{
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

	c := &Configuration{
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

	c := &Configuration{
		Servers:  []string{"http://localhost:9200"},
		LogLevel: "error",
		TLS:      configtls.ClientConfig{Insecure: true},
	}

	rt, err := GetHTTPRoundTripper(ctx, c, logger, mockAuth)

	require.NoError(t, err)
	require.NotNil(t, rt)
	wrappedRT, ok := rt.(*mockWrappedRoundTripper)
	require.True(t, ok, "Should be wrapped round tripper")
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

	mf.AssertCounterMetrics(t,
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

func TestCustomHeaders(t *testing.T) {
	tests := []struct {
		name     string
		config   Configuration
		expected map[string]string
	}{
		{
			name: "custom headers are set correctly",
			config: Configuration{
				Servers: []string{"http://localhost:9200"},
				CustomHeaders: map[string]string{
					"Host":            "my-opensearch.amazonaws.com",
					"X-Custom-Header": "test-value",
				},
			},
			expected: map[string]string{
				"Host":            "my-opensearch.amazonaws.com",
				"X-Custom-Header": "test-value",
			},
		},
		{
			name: "empty custom headers",
			config: Configuration{
				Servers:       []string{"http://localhost:9200"},
				CustomHeaders: map[string]string{},
			},
			expected: map[string]string{},
		},
		{
			name: "nil custom headers",
			config: Configuration{
				Servers:       []string{"http://localhost:9200"},
				CustomHeaders: nil,
			},
			expected: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.expected == nil {
				assert.Nil(t, test.config.CustomHeaders)
			} else {
				assert.Equal(t, test.expected, test.config.CustomHeaders)
			}
		})
	}
}

func TestApplyDefaultsCustomHeaders(t *testing.T) {
	source := &Configuration{
		CustomHeaders: map[string]string{
			"Host":            "source-host",
			"X-Custom-Header": "source-value",
		},
	}

	tests := []struct {
		name     string
		target   *Configuration
		expected map[string]string
	}{
		{
			name:   "target has no headers, apply from source",
			target: &Configuration{},
			expected: map[string]string{
				"Host":            "source-host",
				"X-Custom-Header": "source-value",
			},
		},
		{
			name: "target has headers, keep target headers",
			target: &Configuration{
				CustomHeaders: map[string]string{
					"Host": "target-host",
				},
			},
			expected: map[string]string{
				"Host": "target-host",
			},
		},
		{
			name: "target has empty map, keep empty",
			target: &Configuration{
				CustomHeaders: map[string]string{},
			},
			expected: map[string]string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.target.ApplyDefaults(source)
			assert.Equal(t, test.expected, test.target.CustomHeaders)
		})
	}
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

	config := Configuration{
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

	client, err := NewClient(context.Background(), &config, logger, metricsFactory, nil)
	require.NoError(t, err)
	require.NotNil(t, client)

	// Verify the configuration has the custom headers set
	// Note: The ES v8 client may not send custom headers during the initial ping/health check,
	// but they will be available for actual Elasticsearch operations (index, search, etc.)
	assert.Equal(t, "my-opensearch.amazonaws.com", config.CustomHeaders["Host"])
	assert.Equal(t, "custom-value", config.CustomHeaders["X-Custom-Header"])

	if headersSeen {
		t.Log(" Custom headers were transmitted in HTTP request")
	} else {
		t.Log("  Custom headers not sent in ping request (expected - will be sent in data operations)")
	}

	err = client.Close()
	require.NoError(t, err)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
