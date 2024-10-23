// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerquery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectedErr string
	}{
		{
			name:        "Empty config",
			config:      &Config{},
			expectedErr: "Storage.TracesPrimary: non zero value required",
		},
		{
			name: "Non empty-config",
			config: &Config{
				Storage: Storage{
					TracesPrimary: "some-storage",
				},
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
				assert.Equal(t, tt.expectedErr, err.Error())
			}
		})
	}
}
