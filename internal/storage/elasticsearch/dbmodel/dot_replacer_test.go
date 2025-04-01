// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDotReplacement(t *testing.T) {
	converter := NewDotReplacer("#")
	k := "foo.foo"
	assert.Equal(t, k, converter.ReplaceDotReplacement(converter.ReplaceDot(k)))
}
