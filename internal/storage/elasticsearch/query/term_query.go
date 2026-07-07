// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// TermQuery matches documents where field exactly equals value. It renders to
// the shorthand form {"term": {field: value}}, matching what the storage layer
// previously produced via olivere's TermQuery.
type TermQuery struct {
	field string
	value any
}

// NewTermQuery creates a TermQuery on the given field and value.
func NewTermQuery(field string, value any) *TermQuery {
	return &TermQuery{field: field, value: value}
}

func (q *TermQuery) Source() (any, error) {
	return map[string]any{
		"term": map[string]any{
			q.field: q.value,
		},
	}, nil
}
