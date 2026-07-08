// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchQuerySource(t *testing.T) {
	src, err := NewMatchQuery("process.serviceName", "test-service").Source()
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"match": map[string]any{
			"process.serviceName": map[string]any{"query": "test-service"},
		},
	}, src)
}
