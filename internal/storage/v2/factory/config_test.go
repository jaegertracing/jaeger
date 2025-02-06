// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package factory

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigFromEnv(t *testing.T) {
	f := ConfigFromEnv(&bytes.Buffer{})
	assert.Empty(t, f.TraceWriterTypes)

	t.Setenv(TraceStorageTypeEnvVar, clickhouseStorageType)
	f = ConfigFromEnv(&bytes.Buffer{})
	assert.Len(t, f.TraceWriterTypes, 1)
	assert.Equal(t, clickhouseStorageType, f.TraceWriterTypes[0])
}
