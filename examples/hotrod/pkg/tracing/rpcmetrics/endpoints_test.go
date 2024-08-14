// Copyright (c) 2023 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package rpcmetrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizedEndpoints(t *testing.T) {
	n := newNormalizedEndpoints(1, DefaultNameNormalizer)

	assertLen := func(l int) {
		n.mux.RLock()
		defer n.mux.RUnlock()
		assert.Len(t, n.names, l)
	}

	assert.Equal(t, "ab_cd", n.normalize("ab^cd"), "one translation")
	assert.Equal(t, "ab_cd", n.normalize("ab^cd"), "cache hit")
	assertLen(1)
	assert.Equal(t, "", n.normalize("xys"), "cache overflow")
	assertLen(1)
}

func TestNormalizedEndpointsDoubleLocking(t *testing.T) {
	n := newNormalizedEndpoints(1, DefaultNameNormalizer)
	assert.Equal(t, "ab_cd", n.normalize("ab^cd"), "fill out the cache")
	assert.Equal(t, "", n.normalizeWithLock("xys"), "cache overflow")
}
