// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeServiceName(t *testing.T) {
	assert.Equal(t, "abc", normalizeServiceName("ABC"), "lower case conversion")
	assert.Equal(t, "a_b_c__", normalizeServiceName("a&b%c/:"), "disallowed runes to underscore")
	assert.Equal(t, "a_z_0123456789.", normalizeServiceName("A_Z_0123456789."), "allowed runes")
}
