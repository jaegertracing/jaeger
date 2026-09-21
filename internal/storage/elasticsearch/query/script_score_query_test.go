// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScriptScoreQuery(t *testing.T) {
	query := NewScriptScoreQuery(NewTermQuery("references.traceID", "trace"), "return 1", map[string]any{"field": "traceID"}, 1)
	source, err := query.Source()
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"script_score": map[string]any{
			"query": map[string]any{"term": map[string]any{"references.traceID": "trace"}},
			"script": map[string]any{
				"lang": "painless", "source": "return 1", "params": map[string]any{"field": "traceID"},
			},
			"min_score": 1.0,
		},
	}, source)
}

func TestScriptScoreQueryPropagatesChildError(t *testing.T) {
	_, err := NewScriptScoreQuery(errQuery{}, "return 1", nil, 1).Source()
	require.ErrorIs(t, err, errBadQuery)
}
