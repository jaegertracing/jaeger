// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package normalizer

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestServiceNameReplacer(t *testing.T) {
	assert.Equal(t, "abc", ServiceName("ABC"), "lower case conversion")
	assert.Equal(t, "a_b_c__", ServiceName("a&b%c/:"), "disallowed runes to underscore")
	assert.Equal(t, "a_z_0123456789.", ServiceName("A_Z_0123456789."), "allowed runes")
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
