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
		envVar       string
		expectedType Kind
		expectsError bool
	}{
		// default
		{
			expectedType: Kind("file"),
		},
		// file on both env vars
		{
			env:          "file",
			envVar:       deprecatedSamplingTypeEnvVar,
			expectedType: Kind("file"),
		},
		{
			env:          "file",
			envVar:       SamplingTypeEnvVar,
			expectedType: Kind("file"),
		},
		// static works on the deprecated env var, but fails on the new
		{
			env:          "static",
			envVar:       deprecatedSamplingTypeEnvVar,
			expectedType: Kind("file"),
		},
		{
			env:          "static",
			envVar:       SamplingTypeEnvVar,
			expectsError: true,
		},
		// adaptive on both env vars
		{
			env:          "adaptive",
			envVar:       deprecatedSamplingTypeEnvVar,
			expectedType: Kind("adaptive"),
		},
		{
			env:          "adaptive",
			envVar:       SamplingTypeEnvVar,
			expectedType: Kind("adaptive"),
		},
		// unexpected string on both env vars
		{
			env:          "??",
			envVar:       deprecatedSamplingTypeEnvVar,
			expectsError: true,
		},
		{
			env:          "??",
			envVar:       SamplingTypeEnvVar,
			expectsError: true,
		},
	}

	for _, tc := range tests {
		// clear env
		os.Setenv(SamplingTypeEnvVar, "")
		os.Setenv(deprecatedSamplingTypeEnvVar, "")

		if len(tc.envVar) != 0 {
			err := os.Setenv(tc.envVar, tc.env)
			require.NoError(t, err)
		}

		f, err := FactoryConfigFromEnv(io.Discard)
		if tc.expectsError {
			assert.Error(t, err)
			continue
		}

		require.NoError(t, err)
		assert.Equal(t, tc.expectedType, f.StrategyStoreType)
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
		// static is switched to file
		{
			deprecatedEnvValue: "static",
			expected:           "file",
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
