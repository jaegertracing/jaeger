// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
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

func TestNewClient(t *testing.T) {
	const (
		pwd1       = "password"
		token      = "token"
		serverCert = "../../config/tlscfg/testdata/example-server-cert.pem"
	)
	pwdFile := filepath.Join(t.TempDir(), "pwd")
	require.NoError(t, os.WriteFile(pwdFile, []byte(pwd1), 0o600))
	pwdtokenFile := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(pwdtokenFile, []byte(token), 0o600))
	// copy certs to temp so we can modify them
	certFilePath := copyToTempFile(t, "cert.crt", serverCert)
	defer certFilePath.Close()

	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		assert.Equal(t, http.MethodGet, req.Method)
		res.WriteHeader(http.StatusOK)
		res.Write(mockEsServerResponseWithVersion0)
	}))
	defer testServer.Close()
	testServer1 := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		assert.Equal(t, http.MethodGet, req.Method)
		res.WriteHeader(http.StatusOK)
		res.Write(mockEsServerResponseWithVersion1)
	}))
	defer testServer1.Close()
	testServer2 := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		assert.Equal(t, http.MethodGet, req.Method)
		res.WriteHeader(http.StatusOK)
		res.Write(mockEsServerResponseWithVersion2)
	}))
	defer testServer2.Close()
	testServer8 := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		assert.Equal(t, http.MethodGet, req.Method)
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
				Servers:               []string{testServer.URL},
				AllowTokenFromContext: true,
				Password:              "secret",
				LogLevel:              "debug",
				Username:              "user",
				PasswordFilePath:      "",
				BulkSize:              -1, // disable bulk; we want immediate flush
				Version:               8,
			},
			expectedError: false,
		},
		{
			name: "success with valid configuration and tls enabled",
			config: &Configuration{
				Servers:               []string{testServer.URL},
				AllowTokenFromContext: true,
				Password:              "secret",
				LogLevel:              "debug",
				Username:              "user",
				PasswordFilePath:      "",
				BulkSize:              -1, // disable bulk; we want immediate flush
				Version:               0,
				TLS:                   tlscfg.Options{Enabled: true},
			},
			expectedError: false,
		},
		{
			name: "success with valid configuration and reading token and certicate from file",
			config: &Configuration{
				Servers:               []string{testServer.URL},
				AllowTokenFromContext: true,
				Password:              "secret",
				LogLevel:              "debug",
				Username:              "user",
				PasswordFilePath:      "",
				BulkSize:              -1, // disable bulk; we want immediate flush
				Version:               0,
				TLS:                   tlscfg.Options{Enabled: false, CAPath: certFilePath.Name()},
				TokenFilePath:         pwdtokenFile,
			},
			expectedError: false,
		},
		{
			name: "succes with invalid configuration of version higher than 8",
			config: &Configuration{
				Servers:               []string{testServer8.URL},
				AllowTokenFromContext: true,
				Password:              "secret",
				LogLevel:              "debug",
				Username:              "user",
				PasswordFilePath:      "",
				BulkSize:              -1, // disable bulk; we want immediate flush
				Version:               9,
			},
			expectedError: false,
		},
		{
			name: "success with valid configuration with version 1",
			config: &Configuration{
				Servers:               []string{testServer1.URL},
				AllowTokenFromContext: true,
				Password:              "secret",
				LogLevel:              "debug",
				Username:              "user",
				PasswordFilePath:      "",
				BulkSize:              -1, // disable bulk; we want immediate flush
			},
			expectedError: false,
		},
		{
			name: "success with valid configuration with version 2",
			config: &Configuration{
				Servers:               []string{testServer2.URL},
				AllowTokenFromContext: true,
				Password:              "secret",
				LogLevel:              "debug",
				Username:              "user",
				PasswordFilePath:      "",
				BulkSize:              -1, // disable bulk; we want immediate flush
			},
			expectedError: false,
		},
		{
			name: "success with valid configuration password from file",
			config: &Configuration{
				Servers:               []string{testServer.URL},
				AllowTokenFromContext: true,
				Password:              "",
				LogLevel:              "debug",
				Username:              "user",
				PasswordFilePath:      pwdFile,
				BulkSize:              -1, // disable bulk; we want immediate flush
			},
			expectedError: false,
		},
		{
			name: "fali with configuration password and password from file are set",
			config: &Configuration{
				Servers:               []string{testServer.URL},
				AllowTokenFromContext: true,
				Password:              "secret",
				LogLevel:              "debug",
				Username:              "user",
				PasswordFilePath:      pwdFile,
				BulkSize:              -1, // disable bulk; we want immediate flush
			},
			expectedError: true,
		},
		{
			name: "fail with missing server",
			config: &Configuration{
				Servers:               []string{},
				AllowTokenFromContext: true,
				Password:              "secret",
				LogLevel:              "debug",
				Username:              "user",
				PasswordFilePath:      "",
				BulkSize:              -1, // disable bulk; we want immediate flush
			},
			expectedError: true,
		},
		{
			name: "fail with invalid configuration invalid loglevel",
			config: &Configuration{
				Servers:               []string{testServer.URL},
				AllowTokenFromContext: true,
				Password:              "secret",
				LogLevel:              "invalid",
				Username:              "user",
				PasswordFilePath:      "",
				BulkSize:              -1, // disable bulk; we want immediate flush
			},
			expectedError: true,
		},
		{
			name: "fail with invalid configuration invalid bulkworkers number",
			config: &Configuration{
				Servers:               []string{testServer.URL},
				AllowTokenFromContext: true,
				Password:              "secret",
				LogLevel:              "invalid",
				Username:              "user",
				PasswordFilePath:      "",
				BulkWorkers:           0,
				BulkSize:              -1, // disable bulk; we want immediate flush
			},
			expectedError: true,
		},
		{
			name: "success with valid configuration and info log level",
			config: &Configuration{
				Servers:               []string{testServer.URL},
				AllowTokenFromContext: true,
				Password:              "secret",
				LogLevel:              "info",
				Username:              "user",
				PasswordFilePath:      "",
				BulkSize:              -1, // disable bulk; we want immediate flush
			},
			expectedError: false,
		},
		{
			name: "success with valid configuration and error log level",
			config: &Configuration{
				Servers:               []string{testServer.URL},
				AllowTokenFromContext: true,
				Password:              "secret",
				LogLevel:              "error",
				Username:              "user",
				PasswordFilePath:      "",
				BulkSize:              -1, // disable bulk; we want immediate flush
			},
			expectedError: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger := zap.NewNop()
			metricsFactory := metrics.NullFactory
			config := test.config
			client, err := NewClient(config, logger, metricsFactory)
			if test.expectedError {
				require.Error(t, err)
				require.Nil(t, client)
			} else {
				require.NoError(t, err)
				require.NotNil(t, client)
				err = client.Close()
				require.NoError(t, err)
			}
			err = config.TLS.Close()
			require.NoError(t, err)
		})
	}
}

func TestApplyDefaults(t *testing.T) {
	source := &Configuration{
		RemoteReadClusters:       []string{"cluster1", "cluster2"},
		Username:                 "sourceUser",
		Password:                 "sourcePass",
		Sniffer:                  true,
		MaxSpanAge:               100,
		AdaptiveSamplingLookback: 50,
		Indices: Indices{
			IndexPrefix: "hello",
			Spans: IndexOptions{
				Shards:   5,
				Replicas: 1,
				Priority: 10,
			},
			Services: IndexOptions{
				Shards:   5,
				Replicas: 1,
				Priority: 20,
			},
			Dependencies: IndexOptions{
				Shards:   5,
				Replicas: 1,
				Priority: 30,
			},
			Sampling: IndexOptions{},
		},
		BulkSize:          1000,
		BulkWorkers:       10,
		BulkActions:       100,
		BulkFlushInterval: 30,
		SnifferTLSEnabled: true,
		Tags:              TagsAsFields{AllAsFields: true, DotReplacement: "dot", Include: "include", File: "file"},
		MaxDocCount:       10000,
		LogLevel:          "info",
		SendGetBodyAs:     "json",
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
				Username:           "customUser",
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
				RemoteReadClusters:       []string{"customCluster"},
				Username:                 "customUser",
				Password:                 "sourcePass",
				Sniffer:                  true,
				SnifferTLSEnabled:        true,
				MaxSpanAge:               100,
				AdaptiveSamplingLookback: 50,
				Indices: Indices{
					IndexPrefix: "hello",
					Spans: IndexOptions{
						Shards:   5,
						Replicas: 1,
						Priority: 10,
					},
					Services: IndexOptions{
						Shards:   5,
						Replicas: 1,
						Priority: 20,
					},
					Dependencies: IndexOptions{
						Shards:   5,
						Replicas: 1,
						Priority: 30,
					},
				},
				BulkSize:          1000,
				BulkWorkers:       10,
				BulkActions:       100,
				BulkFlushInterval: 30,
				Tags:              TagsAsFields{AllAsFields: true, DotReplacement: "dot", Include: "include", File: "file"},
				MaxDocCount:       10000,
				LogLevel:          "info",
				SendGetBodyAs:     "json",
			},
		},
		{
			name: "No Defaults Applied",
			target: &Configuration{
				RemoteReadClusters:       []string{"cluster1", "cluster2"},
				Username:                 "sourceUser",
				Password:                 "sourcePass",
				Sniffer:                  true,
				MaxSpanAge:               100,
				AdaptiveSamplingLookback: 50,
				Indices: Indices{
					IndexPrefix: "hello",
					Spans: IndexOptions{
						Shards:   5,
						Replicas: 1,
						Priority: 10,
					},
					Services: IndexOptions{
						Shards:   5,
						Replicas: 1,
						Priority: 20,
					},
					Dependencies: IndexOptions{
						Shards:   5,
						Replicas: 1,
						Priority: 30,
					},
				},
				BulkSize:          1000,
				BulkWorkers:       10,
				BulkActions:       100,
				BulkFlushInterval: 30,
				SnifferTLSEnabled: true,
				Tags:              TagsAsFields{AllAsFields: true, DotReplacement: "dot", Include: "include", File: "file"},
				MaxDocCount:       10000,
				LogLevel:          "info",
				SendGetBodyAs:     "json",
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
		expectedError bool
	}{
		{
			name: "All valid input are set",
			config: &Configuration{
				Servers: []string{"localhost:8000/dummyserver"},
			},
			expectedError: false,
		},
		{
			name:          "no valid input are set",
			config:        &Configuration{},
			expectedError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.config.Validate()
			if test.expectedError {
				require.Error(t, got)
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

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
