// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// MatchQuery matches an analyzed field against value. It renders to the expanded
// form {"match": {field: {"query": value}}}, matching what the storage layer
// previously produced.
type MatchQuery struct {
	field string
	value any
}

// NewMatchQuery creates a MatchQuery on the given field and value.
func NewMatchQuery(field string, value any) *MatchQuery {
	return &MatchQuery{field: field, value: value}
}

func (q *MatchQuery) Source() (any, error) {
	return map[string]any{
		"match": map[string]any{
			q.field: map[string]any{"query": q.value},
		},
	}, nil
}
