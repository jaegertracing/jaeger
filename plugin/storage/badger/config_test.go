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
			cfg:  &Config{},
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
