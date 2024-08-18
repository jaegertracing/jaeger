// Copyright (c) 2023 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package rpcmetrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSimpleNameNormalizer(t *testing.T) {
	n := &SimpleNameNormalizer{
		SafeSets: []SafeCharacterSet{
			&Range{From: 'a', To: 'z'},
			&Char{'-'},
		},
		Replacement: '-',
	}
	assert.Equal(t, "ab-cd", n.Normalize("ab-cd"), "all valid")
	assert.Equal(t, "ab-cd", n.Normalize("ab.cd"), "single mismatch")
	assert.Equal(t, "a--cd", n.Normalize("aB-cd"), "range letter mismatch")
}
