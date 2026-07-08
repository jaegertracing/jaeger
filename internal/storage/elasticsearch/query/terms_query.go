// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// TermsQuery matches documents where field equals any of the given values. It
// renders to {"terms": {field: [values...]}}, matching what the storage layer
// previously produced via olivere's TermsQuery.
type TermsQuery struct {
	field  string
	values []any
}

// NewTermsQuery creates a TermsQuery on the given field and values.
func NewTermsQuery(field string, values ...any) *TermsQuery {
	return &TermsQuery{field: field, values: values}
}

func (q *TermsQuery) Source() (any, error) {
	return map[string]any{"terms": map[string]any{q.field: q.values}}, nil
}
