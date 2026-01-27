// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"testing"
	"time"

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
		{
			name: "invalid config with negative TTL",
			cfg: Configuration{
				Addresses: []string{"localhost:9000"},
				TTL:       -1 * time.Hour,
			},
			wantErr: true,
		},
		{
			name: "valid config with positive TTL",
			cfg: Configuration{
				Addresses: []string{"localhost:9000"},
				TTL:       24 * time.Hour,
			},
			wantErr: false,
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

func TestTTLDays(t *testing.T) {
	tests := []struct {
		name     string
		ttl      time.Duration
		expected int
	}{
		{
			name:     "0 TTL",
			ttl:      0,
			expected: 0,
		},
		{
			name:     "less than 24h",
			ttl:      23 * time.Hour,
			expected: 0,
		},
		{
			name:     "exactly 24h",
			ttl:      24 * time.Hour,
			expected: 1,
		},
		{
			name:     "more than 24h",
			ttl:      48 * time.Hour,
			expected: 2,
		},
		{
			name:     "not a multiple of 24h",
			ttl:      50 * time.Hour,
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Configuration{TTL: tt.ttl}
			require.Equal(t, tt.expected, cfg.TTLDays())
		})
	}
}
