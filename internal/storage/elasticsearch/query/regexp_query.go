// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// RegexpQuery matches a field against a regular expression. It renders to
// {"regexp": {field: {"value": pattern}}}.
type RegexpQuery struct {
	field string
	value string
}

// NewRegexpQuery creates a RegexpQuery on the given field and pattern.
func NewRegexpQuery(field, value string) *RegexpQuery {
	return &RegexpQuery{field: field, value: value}
}

func (q *RegexpQuery) Source() (any, error) {
	return map[string]any{
		"regexp": map[string]any{
			q.field: map[string]any{"value": q.value},
		},
	}, nil
}
