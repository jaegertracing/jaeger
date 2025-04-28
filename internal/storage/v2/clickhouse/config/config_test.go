// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	excepted := Configuration{
		ClickhouseConfig: DefaultClickhouseConfig(),
		ChPoolConfig:     DefaultChPoolConfig(),
	}

	actual := DefaultConfiguration()
	assert.Equal(t, excepted, actual)
}
