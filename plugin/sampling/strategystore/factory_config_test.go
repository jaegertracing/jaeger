// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package strategystore

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFactoryConfigFromEnv(t *testing.T) {
	tests := []struct {
		env          string
		expectedType Kind
		expectsError bool
	}{
		{
			expectedType: Kind("file"),
		},
		{
			env:          "file",
			expectedType: Kind("file"),
		},
		{
			// static is deprecated and maps to file. functionality has not changed
			env:          "static",
			expectedType: Kind("file"),
		},
		{
			env:          "adaptive",
			expectedType: Kind("adaptive"),
		},
		{
			env:          "??",
			expectsError: true,
		},
	}

	for _, tc := range tests {
		// for each test case test both the old and new env vars
		for _, envVar := range []string{SamplingTypeEnvVar, deprecatedSamplingTypeEnvVar} {
			// clear env
			os.Setenv(SamplingTypeEnvVar, "")
			os.Setenv(deprecatedSamplingTypeEnvVar, "")

			err := os.Setenv(envVar, tc.env)
			require.NoError(t, err)

			f, err := FactoryConfigFromEnv(io.Discard)
			if tc.expectsError {
				assert.Error(t, err)
				continue
			}

			require.NoError(t, err)
			assert.Equal(t, tc.expectedType, f.StrategyStoreType)
		}
	}
}

func TestGetStrategyStoreTypeFromEnv(t *testing.T) {
	tests := []struct {
		deprecatedEnvValue string
		currentEnvValue    string
		expected           string
	}{
		// default to file
		{
			expected: "file",
		},
		// current env var works
		{
			currentEnvValue: "foo",
			expected:        "foo",
		},
		// current overrides deprecated
		{
			currentEnvValue:    "foo",
			deprecatedEnvValue: "blerg",
			expected:           "foo",
		},
		// deprecated accepted
		{
			deprecatedEnvValue: "blerg",
			expected:           "blerg",
		},
	}

	for _, tc := range tests {
		err := os.Setenv(SamplingTypeEnvVar, tc.currentEnvValue)
		require.NoError(t, err)
		err = os.Setenv(deprecatedSamplingTypeEnvVar, tc.deprecatedEnvValue)
		require.NoError(t, err)

		actual := getStrategyStoreTypeFromEnv(io.Discard)
		assert.Equal(t, actual, tc.expected)
	}
}
