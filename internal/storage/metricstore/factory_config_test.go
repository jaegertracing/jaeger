// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package metricstore

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFactoryConfigFromEnv(t *testing.T) {
	fc := FactoryConfigFromEnv()
	assert.Empty(t, fc.MetricsStorageType)

	t.Setenv(StorageTypeEnvVar, prometheusStorageType)

	fc = FactoryConfigFromEnv()
	assert.Equal(t, prometheusStorageType, fc.MetricsStorageType)
}
