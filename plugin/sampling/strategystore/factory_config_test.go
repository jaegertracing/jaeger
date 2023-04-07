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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFactoryConfigFromEnv(t *testing.T) {
	tests := []struct {
		name         string
		env          string
		envVar       string
		expectedType Kind
		expectsError bool
	}{
		{
			name:         "default",
			expectedType: Kind("file"),
		},
		{
			name:         "file on deprecatedSamplingTypeEnvVar",
			env:          "file",
			envVar:       deprecatedSamplingTypeEnvVar,
			expectedType: Kind("file"),
		},
		{
			name:         "file on SamplingTypeEnvVar",
			env:          "file",
			envVar:       SamplingTypeEnvVar,
			expectedType: Kind("file"),
		},
		{
			name:         "static works on the deprecatedSamplingTypeEnvVar",
			env:          "static",
			envVar:       deprecatedSamplingTypeEnvVar,
			expectedType: Kind("file"),
		},
		{
			name:         "static fails on the SamplingTypeEnvVar",
			env:          "static",
			envVar:       SamplingTypeEnvVar,
			expectsError: true,
		},
		{
			name:         "adaptive on deprecatedSamplingTypeEnvVar",
			env:          "adaptive",
			envVar:       deprecatedSamplingTypeEnvVar,
			expectedType: Kind("adaptive"),
		},
		{
			name:         "adaptive on SamplingTypeEnvVar",
			env:          "adaptive",
			envVar:       SamplingTypeEnvVar,
			expectedType: Kind("adaptive"),
		},
		{
			name:         "unexpected string on deprecatedSamplingTypeEnvVar",
			env:          "??",
			envVar:       deprecatedSamplingTypeEnvVar,
			expectsError: true,
		},
		{
			name:         "unexpected string on SamplingTypeEnvVar",
			env:          "??",
			envVar:       SamplingTypeEnvVar,
			expectsError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.envVar) != 0 {
				t.Setenv(tc.envVar, tc.env)
			}

			f, err := FactoryConfigFromEnv(io.Discard)
			if tc.expectsError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.expectedType, f.StrategyStoreType)
		})
	}
}

func TestGetStrategyStoreTypeFromEnv(t *testing.T) {
	tests := []struct {
		name               string
		deprecatedEnvValue string
		currentEnvValue    string
		expected           string
	}{
		{
			name:     "default to file",
			expected: "file",
		},
		{
			name:            "current env var works",
			currentEnvValue: "foo",
			expected:        "foo",
		},
		{
			name:               "current overrides deprecated",
			currentEnvValue:    "foo",
			deprecatedEnvValue: "blerg",
			expected:           "foo",
		},
		{
			name:               "deprecated accepted",
			deprecatedEnvValue: "blerg",
			expected:           "blerg",
		},
		{
			name:               "static is switched to file",
			deprecatedEnvValue: "static",
			expected:           "file",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(SamplingTypeEnvVar, tc.currentEnvValue)
			t.Setenv(deprecatedSamplingTypeEnvVar, tc.deprecatedEnvValue)

			actual := getStrategyStoreTypeFromEnv(io.Discard)
			assert.Equal(t, actual, tc.expected)
		})
	}
}
