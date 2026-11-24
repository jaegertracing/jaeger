// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigFile(t *testing.T) {
	tests := []struct {
		name        string
		yamlConfig  string
		env         map[string]string
		expectError string
		validate    func(*testing.T, *Config)
	}{
		{
			name: "valid memory backend",
			yamlConfig: `
grpc:
  endpoint: :17271
storage:
  backends:
    default-storage:
      memory:
        max_traces: 50000
`,
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, ":17271", cfg.GRPC.NetAddr.Endpoint)
				assert.Len(t, cfg.Storage.TraceBackends, 1)
				assert.NotNil(t, cfg.Storage.TraceBackends["default-storage"].Memory)
				assert.EqualValues(t, 50000, cfg.Storage.TraceBackends["default-storage"].Memory.MaxTraces)
				assert.Equal(t, "default-storage", cfg.GetStorageName())
			},
		},
		{
			name: "memory max_traces that is not a number",
			yamlConfig: `
grpc:
  endpoint: :17271
storage:
  backends:
    default-storage:
      memory:
        max_traces: NOT-A-NUMBER
`,
			expectError: "max_traces",
		},
		{
			name: "valid badger backend",
			yamlConfig: `
grpc:
  endpoint: :17272
storage:
  backends:
    badger-storage:
      badger:
        directories:
          keys: /tmp/test-keys
          values: /tmp/test-values
        ephemeral: true
`,
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, ":17272", cfg.GRPC.NetAddr.Endpoint)
				assert.Len(t, cfg.Storage.TraceBackends, 1)
				assert.NotNil(t, cfg.Storage.TraceBackends["badger-storage"].Badger)
				assert.Equal(t, "badger-storage", cfg.GetStorageName())
			},
		},
		{
			// The backend's Unmarshal hook supplies its defaults, and a configoptional field is
			// decoded: neither happened when the file was decoded with viper.
			name: "elasticsearch backend gets its defaults and optional fields",
			yamlConfig: `
grpc:
  endpoint: :17271
storage:
  backends:
    es-storage:
      elasticsearch:
        auth:
          basic:
            username: jaeger
`,
			validate: func(t *testing.T, cfg *Config) {
				es := cfg.Storage.TraceBackends["es-storage"].Elasticsearch
				require.NotNil(t, es)
				assert.Equal(t, []string{"http://127.0.0.1:9200"}, es.Servers, "default from DefaultConfig")
				assert.EqualValues(t, 5, es.Indices.Spans.Shards, "default from DefaultConfig")
				require.True(t, es.Authentication.BasicAuthentication.HasValue())
				assert.Equal(t, "jaeger", es.Authentication.BasicAuthentication.Get().Username)
			},
		},
		{
			name: "environment variables are expanded",
			yamlConfig: `
grpc:
  endpoint: ${env:REMOTE_STORAGE_TEST_ENDPOINT}
storage:
  backends:
    default-storage:
      memory:
        max_traces: ${env:REMOTE_STORAGE_TEST_MAX_TRACES:-7}
`,
			env: map[string]string{"REMOTE_STORAGE_TEST_ENDPOINT": ":17999"},
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, ":17999", cfg.GRPC.NetAddr.Endpoint)
				assert.EqualValues(t, 7, cfg.Storage.TraceBackends["default-storage"].Memory.MaxTraces)
			},
		},
		{
			// The backend's own Validate runs, as it does in the main binary.
			name: "backend validation error surfaces",
			yamlConfig: `
grpc:
  endpoint: :17271
storage:
  backends:
    es-storage:
      elasticsearch:
        log_level: trace
`,
			expectError: `unrecognized log_level "trace"`,
		},
		{
			name: "unknown key rejected",
			yamlConfig: `
grpc:
  endpoint: :17271
storage:
  backends:
    default-storage:
      memory:
        max_traces: 1000
        max_spans: 1000
`,
			expectError: "max_spans",
		},
		{
			name: "service sections decode over their defaults",
			yamlConfig: `
admin:
  endpoint: :18270
logging:
  level: debug
metrics:
  backend: none
storage:
  backends:
    default-storage:
      memory:
        max_traces: 1000
`,
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, ":18270", cfg.Service.Admin.Endpoint)
				assert.Equal(t, "debug", cfg.Service.Logging.Level)
				assert.Equal(t, "json", cfg.Service.Logging.Encoding, "default kept")
				assert.Equal(t, "none", cfg.Service.Metrics.Backend)
				assert.Equal(t, ":17271", cfg.GRPC.NetAddr.Endpoint, "default kept")
			},
		},
		{
			name: "unknown top-level key rejected",
			yamlConfig: `
log-level: debug
storage:
  backends:
    default-storage:
      memory:
        max_traces: 1000
`,
			expectError: "log-level",
		},
		{
			name: "section that is not a mapping",
			yamlConfig: `
grpc:
  endpoint: :17271
storage: not-a-mapping
`,
			expectError: "storage",
		},
		{
			name: "unknown key inside the admin section is rejected",
			yamlConfig: `
admin:
  endpont: 127.0.0.1:18000
`,
			expectError: "endpont",
		},
		{
			name: "environment reference without a scheme reads the environment",
			yamlConfig: `
grpc:
  endpoint: ${REMOTE_STORAGE_TEST_ENDPOINT}
`,
			env:      map[string]string{"REMOTE_STORAGE_TEST_ENDPOINT": ":17998"},
			validate: func(t *testing.T, cfg *Config) { assert.Equal(t, ":17998", cfg.GRPC.NetAddr.Endpoint) },
		},
		{
			name: "storage section without a backend",
			yamlConfig: `
grpc:
  endpoint: :17271
storage:
  backends: {}
`,
			expectError: "at least one storage backend is required",
		},
		{
			name: "file without a storage section keeps the default memory backend",
			yamlConfig: `
logging:
  level: debug
`,
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "debug", cfg.Service.Logging.Level)
				assert.Equal(t, DefaultConfig().Storage, cfg.Storage)
			},
		},
		{
			name: "empty backend configuration",
			yamlConfig: `
grpc:
  endpoint: :17271
storage:
  backends:
    empty-storage: {}
`,
			expectError: "empty configuration",
		},
		{
			name: "multiple backends should fail",
			yamlConfig: `
grpc:
  endpoint: :17271
storage:
  backends:
    memory-storage:
      memory:
        max_traces: 10000
    another-storage:
      memory:
        max_traces: 20000
`,
			expectError: "remote-storage only supports a single storage backend",
		},
		{
			name: "with multi-tenancy enabled",
			yamlConfig: `
grpc:
  endpoint: :17271
multi_tenancy:
  enabled: true
  header: x-tenant
  tenants:
    - tenant1
    - tenant2
storage:
  backends:
    default-storage:
      memory:
        max_traces: 10000
`,
			validate: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.Tenancy.Enabled)
				assert.Equal(t, "x-tenant", cfg.Tenancy.Header)
				assert.Equal(t, []string{"tenant1", "tenant2"}, cfg.Tenancy.Tenants)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			configFile := filepath.Join(t.TempDir(), "config.yaml")
			require.NoError(t, os.WriteFile(configFile, []byte(tt.yamlConfig), 0o600))

			cfg, err := LoadConfigFile(context.Background(), configFile)

			if tt.expectError != "" {
				require.ErrorContains(t, err, tt.expectError)
			} else {
				require.NoError(t, err)
				require.NotNil(t, cfg)
			}
			if tt.validate != nil {
				tt.validate(t, cfg)
			}
		})
	}
}

func TestLoadConfigFileMissing(t *testing.T) {
	_, err := LoadConfigFile(context.Background(), filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	require.ErrorContains(t, err, "does-not-exist.yaml")
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	require.NotNil(t, cfg)
	require.Equal(t, ":17270", cfg.Service.Admin.Endpoint)
	require.Equal(t, "info", cfg.Service.Logging.Level)
	require.Equal(t, "prometheus", cfg.Service.Metrics.Backend)
	require.Equal(t, ":17271", cfg.GRPC.NetAddr.Endpoint)
	require.Len(t, cfg.Storage.TraceBackends, 1)
	require.NotNil(t, cfg.Storage.TraceBackends["memory"].Memory)
	require.Equal(t, "memory", cfg.GetStorageName())
}

// TestLoadShippedConfigFiles loads the example files next to the binary, which the README
// points users at, so that they keep decoding as the loader changes.
func TestLoadShippedConfigFiles(t *testing.T) {
	for _, file := range []string{"../config.yaml", "../config-badger.yaml"} {
		t.Run(filepath.Base(file), func(t *testing.T) {
			cfg, err := LoadConfigFile(context.Background(), file)
			require.NoError(t, err)
			assert.Equal(t, ":17271", cfg.GRPC.NetAddr.Endpoint)
			assert.Len(t, cfg.Storage.TraceBackends, 1)
		})
	}
}
