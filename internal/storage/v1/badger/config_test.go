// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package badger

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidate_DoesNotReturnErrorWhenValid(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
	}{
		{
			name: "non-required fields not set",
			cfg: &Config{
				TTL: TTL{
					Spans: time.Second,
				},
				MaintenanceInterval:   time.Second,
				MetricsUpdateInterval: time.Second,
			},
		},
		{
			name: "default config",
			cfg:  DefaultConfig(),
		},
		{
			name: "all fields are set",
			cfg: &Config{
				TTL: TTL{
					Spans: time.Second,
				},
				Directories: Directories{
					Keys:   "some-key-directory",
					Values: "some-values-directory",
				},
				Ephemeral:             false,
				SyncWrites:            false,
				MaintenanceInterval:   time.Second,
				MetricsUpdateInterval: time.Second,
				ReadOnly:              false,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.cfg.Validate()
			require.NoError(t, err)
		})
	}
}

func TestValidate_ReturnsErrorForNonPositiveDurations(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr error
	}{
		{
			name:    "zero spans TTL",
			mutate:  func(c *Config) { c.TTL.Spans = 0 },
			wantErr: errNonPositiveSpansTTL,
		},
		{
			name:    "negative spans TTL",
			mutate:  func(c *Config) { c.TTL.Spans = -time.Hour },
			wantErr: errNonPositiveSpansTTL,
		},
		{
			name:    "zero maintenance interval",
			mutate:  func(c *Config) { c.MaintenanceInterval = 0 },
			wantErr: errNonPositiveMaintenanceInterval,
		},
		{
			name:    "negative maintenance interval",
			mutate:  func(c *Config) { c.MaintenanceInterval = -time.Second },
			wantErr: errNonPositiveMaintenanceInterval,
		},
		{
			name:    "zero metrics update interval",
			mutate:  func(c *Config) { c.MetricsUpdateInterval = 0 },
			wantErr: errNonPositiveMetricsUpdateInterval,
		},
		{
			name:    "negative metrics update interval",
			mutate:  func(c *Config) { c.MetricsUpdateInterval = -time.Second },
			wantErr: errNonPositiveMetricsUpdateInterval,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := DefaultConfig()
			test.mutate(cfg)
			require.ErrorIs(t, cfg.Validate(), test.wantErr)
		})
	}
}
