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

func TestValidate_NonPositiveIntervals(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *Config
		expectedErr error
	}{
		{
			name: "zero maintenance interval",
			cfg: &Config{
				MaintenanceInterval:   0,
				MetricsUpdateInterval: time.Second,
			},
			expectedErr: ErrMaintenanceInterval,
		},
		{
			name: "negative maintenance interval",
			cfg: &Config{
				MaintenanceInterval:   -1 * time.Second,
				MetricsUpdateInterval: time.Second,
			},
			expectedErr: ErrMaintenanceInterval,
		},
		{
			name: "zero metrics update interval",
			cfg: &Config{
				MaintenanceInterval:   time.Minute,
				MetricsUpdateInterval: 0,
			},
			expectedErr: ErrMetricsUpdateInterval,
		},
		{
			name: "negative metrics update interval",
			cfg: &Config{
				MaintenanceInterval:   time.Minute,
				MetricsUpdateInterval: -5 * time.Second,
			},
			expectedErr: ErrMetricsUpdateInterval,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.cfg.Validate()
			require.ErrorIs(t, err, test.expectedErr)
		})
	}
}
