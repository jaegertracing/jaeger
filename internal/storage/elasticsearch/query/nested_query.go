// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// NestedQuery runs an inner query against a nested-field path. It renders to
// {"nested": {"path": path, "query": <inner>}}.
type NestedQuery struct {
	path  string
	query Query
}

// NewNestedQuery creates a NestedQuery over path running the given inner query.
func NewNestedQuery(path string, query Query) *NestedQuery {
	return &NestedQuery{path: path, query: query}
}

func (q *NestedQuery) Source() (any, error) {
	inner, err := q.query.Source()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"nested": map[string]any{
			"path":  q.path,
			"query": inner,
		},
	}, nil
}
