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

func TestValidate_ReturnsErrorWhenInvalid(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *Config
		expectedErr error
	}{
		{
			name: "maintenance interval is zero",
			cfg: &Config{
				MaintenanceInterval:   0,
				MetricsUpdateInterval: time.Second,
			},
			expectedErr: errMaintenanceInterval,
		},
		{
			name: "maintenance interval is negative",
			cfg: &Config{
				MaintenanceInterval:   -time.Second,
				MetricsUpdateInterval: time.Second,
			},
			expectedErr: errMaintenanceInterval,
		},
		{
			name: "metrics update interval is zero",
			cfg: &Config{
				MaintenanceInterval:   time.Second,
				MetricsUpdateInterval: 0,
			},
			expectedErr: errMetricsUpdateInterval,
		},
		{
			name: "metrics update interval is negative",
			cfg: &Config{
				MaintenanceInterval:   time.Second,
				MetricsUpdateInterval: -time.Second,
			},
			expectedErr: errMetricsUpdateInterval,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			require.ErrorIs(t, err, tt.expectedErr)
		})
	}
}
