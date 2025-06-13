// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

var _ storage.MetricStoreFactory = new(Factory)

// mockESServerResponse simulates a successful Elasticsearch version response.
var mockESServerResponse = []byte(`
{
    "version": {
       "number": "6.8.0"
    }
}
`)

// setupMockServer creates a mock HTTP server with the specified response and status code.
func setupMockServer(t *testing.T, response []byte, statusCode int) *httptest.Server {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write(response)
	}))
	require.NotNil(t, mockServer)
	t.Cleanup(mockServer.Close)

	return mockServer
}

// newTestFactoryConfig creates a default configuration for testing.
func newTestFactoryConfig(serverURL string) config.Configuration {
	return config.Configuration{
		Servers:  []string{serverURL},
		LogLevel: "debug",
	}
}

func TestCreateMetricsReader(t *testing.T) {
	server := setupMockServer(t, mockESServerResponse, http.StatusOK)
	cfg := newTestFactoryConfig(server.URL)
	f, err := NewFactory(context.Background(), cfg, telemetry.NoopSettings())
	require.NoError(t, err)
	require.NotNil(t, f)
	defer require.NoError(t, f.Close())

	reader, err := f.CreateMetricsReader()
	require.NoError(t, err)
	assert.NotNil(t, reader)
}

func TestNewFactory(t *testing.T) {
	mockServer := setupMockServer(t, mockESServerResponse, http.StatusOK)
	tests := []struct {
		name        string
		cfg         config.Configuration
		response    []byte
		statusCode  int
		expectedErr bool
	}{
		{
			name:        "valid config",
			cfg:         newTestFactoryConfig(mockServer.URL),
			expectedErr: false,
		},
		{
			name: "invalid config - no servers",
			cfg: config.Configuration{
				Servers: []string{},
			},
			expectedErr: true,
		},
		{
			name:        "invalid config - malformed server URL",
			cfg:         newTestFactoryConfig("://malformed-url"),
			expectedErr: true,
		},
		{
			name:        "ping failure for version detection",          // New situation to test error from create es client
			cfg:         newTestFactoryConfig("http://localhost:9090"), // Overridden by mock server
			response:    []byte(`{"error": "internal server error"}`),
			statusCode:  http.StatusInternalServerError,
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.response != nil && tt.statusCode != 0 {
				server := setupMockServer(t, tt.response, tt.statusCode)
				tt.cfg.Servers = []string{server.URL}
			}
			f, err := NewFactory(context.Background(), tt.cfg, telemetry.NoopSettings())

			if tt.expectedErr {
				require.Error(t, err)
				require.Nil(t, f)
			} else {
				require.NoError(t, err)
				require.NotNil(t, f)
				require.NoError(t, f.Close())
			}
		})
	}
}
