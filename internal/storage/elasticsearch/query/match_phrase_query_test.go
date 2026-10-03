// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchPhraseQuerySource(t *testing.T) {
	src, err := NewMatchPhraseQuery("tag.input.text", "refund policy").Source()
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"match_phrase": map[string]any{
			"tag.input.text": map[string]any{"query": "refund policy"},
		},
	}, src)
}
