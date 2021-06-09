// Copyright (c) 2021 The Jaeger Authors.
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

package metrics

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func clearEnv(t *testing.T) {
	err := os.Setenv(StorageTypeEnvVar, "")
	require.NoError(t, err)
}

func TestFactoryConfigFromEnv(t *testing.T) {
	clearEnv(t)
	defer clearEnv(t)

	fc := FactoryConfigFromEnv()
	assert.Empty(t, fc.MetricsStorageType)

	err := os.Setenv(StorageTypeEnvVar, prometheusStorageType)
	require.NoError(t, err)

	fc = FactoryConfigFromEnv()
	assert.Equal(t, prometheusStorageType, fc.MetricsStorageType)
}
