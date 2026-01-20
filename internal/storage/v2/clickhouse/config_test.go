// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Configuration
		wantErr bool
	}{
		{
			name: "valid config with native protocol",
			cfg: Configuration{
				Protocol:  "native",
				Addresses: []string{"localhost:9000"},
			},
			wantErr: false,
		},
		{
			name: "valid config with http protocol",
			cfg: Configuration{
				Protocol:  "http",
				Addresses: []string{"localhost:8123"},
			},
			wantErr: false,
		},
		{
			name: "valid config with empty protocol",
			cfg: Configuration{
				Addresses: []string{"localhost:9000"},
			},
			wantErr: false,
		},
		{
			name: "valid config with multiple addresses",
			cfg: Configuration{
				Protocol:  "native",
				Addresses: []string{"localhost:9000", "localhost:9001"},
			},
			wantErr: false,
		},
		{
			name: "invalid config with unsupported protocol",
			cfg: Configuration{
				Protocol:  "grpc",
				Addresses: []string{"localhost:9000"},
			},
			wantErr: true,
		},
		{
			name: "invalid config with empty addresses",
			cfg: Configuration{
				Protocol:  "native",
				Addresses: []string{},
			},
			wantErr: true,
		},
		{
			name: "invalid config with nil addresses",
			cfg: Configuration{
				Protocol: "native",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfigurationApplyDefaults(t *testing.T) {
	config := &Configuration{}
	config.applyDefaults()

	require.Equal(t, defaultProtocol, config.Protocol)
	require.Equal(t, defaultDatabase, config.Database)
	require.Equal(t, defaultSearchDepth, config.DefaultSearchDepth)
	require.Equal(t, defaultMaxSearchDepth, config.MaxSearchDepth)
}

func TestConfiguration_Validate_TTL(t *testing.T) {
	tests := []struct {
		name        string
		ttl         time.Duration
		expectError bool
	}{
		{
			name:        "Zero TTL (Disabled) is valid",
			ttl:         0,
			expectError: false,
		},
		{
			name:        "Positive TTL is valid",
			ttl:         1 * time.Hour,
			expectError: false,
		},
		{
			name:        "Negative TTL is invalid",
			ttl:         -1 * time.Hour,
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := &Configuration{
				Addresses: []string{"localhost:9000"}, // Required field
				SpansTTL:  test.ttl,
			}
			err := cfg.Validate()
			if test.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "spans_ttl must be a positive duration")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
