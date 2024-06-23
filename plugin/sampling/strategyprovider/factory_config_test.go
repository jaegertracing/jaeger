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

package strategyprovider

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
