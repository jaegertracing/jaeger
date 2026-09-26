// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name        string
		config      *Configuration
		expectedErr error
	}{
		{
			name: "valid max traces",
			config: &Configuration{
				MaxTraces: 1,
			},
			expectedErr: nil,
		},
		{
			name:        "zero max traces",
			config:      &Configuration{},
			expectedErr: errInvalidMaxTraces,
		},
		{
			name: "negative max traces",
			config: &Configuration{
				MaxTraces: -1,
			},
			expectedErr: errInvalidMaxTraces,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.config.Validate()
			if test.expectedErr != nil {
				require.ErrorIs(t, err, test.expectedErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
