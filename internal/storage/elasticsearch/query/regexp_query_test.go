// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegexpQuerySource(t *testing.T) {
	src, err := NewRegexpQuery("tags.value", "200").Source()
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"regexp": map[string]any{
			"tags.value": map[string]any{"value": "200"},
		},
	}, src)
}
