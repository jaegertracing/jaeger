// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// ScriptScoreQuery evaluates script for documents selected by query. A score below
// minScore is excluded before aggregations, which makes it suitable for an exact
// source-aware predicate after an indexed candidate query.
type ScriptScoreQuery struct {
	query    Query
	source   string
	params   map[string]any
	minScore float64
}

// NewScriptScoreQuery creates a source-aware query whose positive scores are
// retained by min_score. The script is deliberately supplied as parameters rather
// than interpolated so constants do not change the script's source.
func NewScriptScoreQuery(query Query, source string, params map[string]any, minScore float64) *ScriptScoreQuery {
	return &ScriptScoreQuery{query: query, source: source, params: params, minScore: minScore}
}

func (q *ScriptScoreQuery) Source() (any, error) {
	inner, err := q.query.Source()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"script_score": map[string]any{
			"query": inner,
			"script": map[string]any{
				"lang":   "painless",
				"source": q.source,
				"params": q.params,
			},
			"min_score": q.minScore,
		},
	}, nil
}
