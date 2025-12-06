// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigFromViper(t *testing.T) {
	tests := []struct {
		name        string
		yamlConfig  string
		expectError bool
		validate    func(*testing.T, *Config)
	}{
		{
			name: "valid memory backend",
			yamlConfig: `
grpc:
  host-port: :17271
storage:
  backends:
    default-storage:
      memory:
        max_traces: 50000
`,
			expectError: false,
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, ":17271", cfg.GRPC.HostPort)
				assert.Len(t, cfg.Storage.TraceBackends, 1)
				assert.NotNil(t, cfg.Storage.TraceBackends["default-storage"].Memory)
				assert.Equal(t, 50000, cfg.Storage.TraceBackends["default-storage"].Memory.MaxTraces)
				assert.Equal(t, "default-storage", cfg.GetStorageName())
			},
		},
		{
			name: "valid badger backend",
			yamlConfig: `
grpc:
  host-port: :17272
storage:
  backends:
    badger-storage:
      badger:
        directories:
          keys: /tmp/test-keys
          values: /tmp/test-values
        ephemeral: true
`,
			expectError: false,
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, ":17272", cfg.GRPC.HostPort)
				assert.Len(t, cfg.Storage.TraceBackends, 1)
				assert.NotNil(t, cfg.Storage.TraceBackends["badger-storage"].Badger)
				assert.Equal(t, "badger-storage", cfg.GetStorageName())
			},
		},
		{
			name: "missing storage backend",
			yamlConfig: `
grpc:
  host-port: :17271
storage:
  backends: {}
`,
			expectError: true,
		},
		{
			name: "empty backend configuration",
			yamlConfig: `
grpc:
  host-port: :17271
storage:
  backends:
    empty-storage: {}
`,
			expectError: true,
		},
		{
			name: "multiple backends",
			yamlConfig: `
grpc:
  host-port: :17271
storage:
  backends:
    memory-storage:
      memory:
        max_traces: 10000
    another-storage:
      memory:
        max_traces: 20000
`,
			expectError: false,
			validate: func(t *testing.T, cfg *Config) {
				assert.Len(t, cfg.Storage.TraceBackends, 2)
				// GetStorageName should return one of them (first in iteration order)
				assert.Contains(t, []string{"memory-storage", "another-storage"}, cfg.GetStorageName())
			},
		},
		{
			name: "with multi-tenancy enabled",
			yamlConfig: `
grpc:
  host-port: :17271
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
			expectError: false,
			validate: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.Tenancy.Enabled)
				assert.Equal(t, "x-tenant", cfg.Tenancy.Header)
				assert.Equal(t, []string{"tenant1", "tenant2"}, cfg.Tenancy.Tenants)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary config file
			tmpDir := t.TempDir()
			configFile := filepath.Join(tmpDir, "config.yaml")
			err := os.WriteFile(configFile, []byte(tt.yamlConfig), 0o600)
			require.NoError(t, err)

			// Load config with Viper
			v := viper.New()
			v.SetConfigFile(configFile)
			err = v.ReadInConfig()
			require.NoError(t, err)

			// Load config from Viper
			cfg, err := LoadConfigFromViper(v)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, cfg)
				if tt.validate != nil {
					tt.validate(t, cfg)
				}
			}
		})
	}
}
