// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectError bool
	}{
		{
			name:        "Empty config - missing storage",
			config:      &Config{},
			expectError: true,
		},
		{
			name: "Valid config",
			config: &Config{
				Storage: Storage{
					Traces: "some-storage",
				},
				MaxSpanDetailsPerRequest: 20,
				MaxSearchResults:         100,
			},
			expectError: false,
		},
		{
			name: "Invalid MaxSpanDetailsPerRequest (too high)",
			config: &Config{
				Storage: Storage{
					Traces: "some-storage",
				},
				MaxSpanDetailsPerRequest: 101,
				MaxSearchResults:         100,
			},
			expectError: true,
		},
		{
			name: "Invalid MaxSearchResults (too high)",
			config: &Config{
				Storage: Storage{
					Traces: "some-storage",
				},
				MaxSpanDetailsPerRequest: 20,
				MaxSearchResults:         1001,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
