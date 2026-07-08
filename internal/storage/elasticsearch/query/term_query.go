// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// TermQuery matches documents where field exactly equals value. Without a boost
// it renders to the shorthand form {"term": {field: value}}; with one it renders
// to the expanded {"term": {field: {"value": value, "boost": boost}}} — both
// matching what the storage layer previously produced via olivere's TermQuery.
type TermQuery struct {
	field string
	value any
	boost *float64
}

// NewTermQuery creates a TermQuery on the given field and value.
func NewTermQuery(field string, value any) *TermQuery {
	return &TermQuery{field: field, value: value}
}

// Boost weights this term's contribution to the relevance score, switching the
// query to its expanded form.
func (q *TermQuery) Boost(boost float64) *TermQuery {
	q.boost = &boost
	return q
}

func (q *TermQuery) Source() (any, error) {
	inner := any(q.value)
	if q.boost != nil {
		inner = map[string]any{"value": q.value, "boost": *q.boost}
	}
	return map[string]any{
		"term": map[string]any{
			q.field: inner,
		},
	}, nil
}
