// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package metafactory

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFactoryConfigFromEnv(t *testing.T) {
	tests := []struct {
		name         string
		env          string
		expectedType Kind
		expectsError bool
	}{
		{
			name:         "default",
			expectedType: Kind("file"),
		},
		{
			name:         "file on SamplingTypeEnvVar",
			env:          "file",
			expectedType: Kind("file"),
		},
		{
			name:         "old value 'static' fails on the SamplingTypeEnvVar",
			env:          "static",
			expectsError: true,
		},
		{
			name:         "adaptive on SamplingTypeEnvVar",
			env:          "adaptive",
			expectedType: Kind("adaptive"),
		},
		{
			name:         "unexpected string on SamplingTypeEnvVar",
			env:          "??",
			expectsError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.env != "" {
				t.Setenv(SamplingTypeEnvVar, tc.env)
			}

			f, err := FactoryConfigFromEnv()
			if tc.expectsError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.expectedType, f.StrategyStoreType)
		})
	}
}
