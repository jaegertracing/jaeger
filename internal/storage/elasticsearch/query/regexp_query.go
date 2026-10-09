// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// RegexpQuery matches a field against a regular expression. It renders to
// {"regexp": {field: {"value": pattern}}}, matching what the storage layer
// previously produced. When flags are specified, it renders to
// {"regexp": {field: {"value": pattern, "flags": flags}}}.
type RegexpQuery struct {
	field string
	value string
	flags string
}

// NewRegexpQuery creates a RegexpQuery on the given field and pattern.
func NewRegexpQuery(field, value string) *RegexpQuery {
	return &RegexpQuery{field: field, value: value}
}

// Flags picks which of Lucene's optional syntax extensions the pattern may use.
// Leaving it unset is not the same as switching them off: Elasticsearch reads an
// absent flags field as ALL, so &, @, ~, <n-m>, # and <identifier> stay live and a
// pattern meant as text can be read as an operator. "NONE" is the way to turn them off.
func (q *RegexpQuery) Flags(flags string) *RegexpQuery {
	q.flags = flags
	return q
}

func (q *RegexpQuery) Source() (any, error) {
	inner := map[string]any{"value": q.value}
	if q.flags != "" {
		inner["flags"] = q.flags
	}
	return map[string]any{
		"regexp": map[string]any{
			q.field: inner,
		},
	}, nil
}
