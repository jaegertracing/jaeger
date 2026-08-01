// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// NestedQuery runs an inner query against a nested-field path. It renders to
// {"nested": {"path": path, "query": <inner>}}, matching what the storage layer
// previously produced.
type NestedQuery struct {
	path           string
	query          Query
	ignoreUnmapped bool
}

// NewNestedQuery creates a NestedQuery over path running the given inner query.
func NewNestedQuery(path string, query Query) *NestedQuery {
	return &NestedQuery{path: path, query: query}
}

// IgnoreUnmapped makes the query match no documents instead of failing when its
// nested path is absent from an index mapping.
func (q *NestedQuery) IgnoreUnmapped(ignoreUnmapped bool) *NestedQuery {
	q.ignoreUnmapped = ignoreUnmapped
	return q
}

func (q *NestedQuery) Source() (any, error) {
	inner, err := q.query.Source()
	if err != nil {
		return nil, err
	}
	nested := map[string]any{
		"path":  q.path,
		"query": inner,
	}
	if q.ignoreUnmapped {
		nested["ignore_unmapped"] = true
	}
	return map[string]any{"nested": nested}, nil
}
