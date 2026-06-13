// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package remotestorage

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectedErr string
	}{
		{
			name:        "Empty config",
			config:      &Config{},
			expectedErr: "Storage: non zero value required",
		},
		{
			name: "Non empty-config",
			config: &Config{
				Storage: "some-storage",
			},
			expectedErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectedErr == "" {
				require.NoError(t, err)
			} else {
				require.Equal(t, tt.expectedErr, err.Error())
			}
		})
	}
}
