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
			expectedType: Kind("static"),
		},
		{
			env:          "static",
			expectedType: Kind("static"),
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
		err := os.Setenv(SamplingTypeEnvVar, tc.env)
		require.NoError(t, err)

		f, err := FactoryConfigFromEnv()
		if tc.expectsError {
			assert.Error(t, err)
			continue
		}

		require.NoError(t, err)
		assert.Equal(t, tc.expectedType, f.StrategyStoreType)
	}
}
