// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// ExistsQuery matches documents that have any value for a field. It renders to
// {"exists": {"field": field}}, matching what the storage layer previously
// produced via olivere's ExistsQuery.
type ExistsQuery struct {
	field string
}

// NewExistsQuery creates an ExistsQuery on the given field.
func NewExistsQuery(field string) *ExistsQuery {
	return &ExistsQuery{field: field}
}

func (q *ExistsQuery) Source() (any, error) {
	return map[string]any{"exists": map[string]any{"field": q.field}}, nil
}
