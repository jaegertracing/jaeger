// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSamplerTypeToString(t *testing.T) {
	for kStr, vEnum := range toSamplerType {
		assert.Equal(t, kStr, vEnum.String())
	}
	assert.Equal(t, "", SamplerType(-1).String())
}
