// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
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

// mockESServerResponse simulates a basic Elasticsearch version response.
var mockESServerResponse = []byte(`
{
    "version": {
       "number": "6.8.0"
    }
}
`)

func setupMockServer(t *testing.T) (*httptest.Server, func()) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(mockESServerResponse)
	}))
	require.NotNil(t, mockServer)

	return mockServer, mockServer.Close
}

func TestCreateMetricsReader(t *testing.T) {
	mockServer, closeServer := setupMockServer(t)
	defer closeServer()
	cfg := config.Configuration{
		Servers:  []string{mockServer.URL},
		LogLevel: "debug",
	}
	f, err := NewFactory(cfg, telemetry.NoopSettings())
	require.NoError(t, err)
	require.NotNil(t, f)

	reader, err := f.CreateMetricsReader()
	require.NoError(t, err)
	assert.NotNil(t, reader)
	require.NoError(t, f.Close())
}

func TestCreateMetricsReaderError(t *testing.T) {
	cfg := config.Configuration{
		Servers: []string{""},
	}
	f := Factory{
		config: cfg,
		telset: telemetry.NoopSettings(),
	}
	require.NotNil(t, f)

	reader, err := f.CreateMetricsReader()

	require.Error(t, err)
	assert.Nil(t, reader)
	require.NoError(t, f.Close())
}

func TestNewFactory(t *testing.T) {
	mockServer, closeServer := setupMockServer(t)
	defer closeServer()
	tests := []struct {
		name        string
		cfg         config.Configuration
		expectedErr bool
	}{
		{
			name: "valid config",
			cfg: config.Configuration{
				Servers:  []string{mockServer.URL},
				LogLevel: "debug",
			},
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
			name: "invalid config - malformed server URL",
			cfg: config.Configuration{
				Servers: []string{"://malformed-url"}, // Malformed URL
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			telset := telemetry.NoopSettings()
			f, err := NewFactory(tt.cfg, telset)

			if tt.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NoError(t, f.Close())
			}
		})
	}
}
