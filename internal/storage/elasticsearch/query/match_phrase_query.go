// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// MatchPhraseQuery matches an analyzed field against a phrase. Unlike MatchQuery,
// which tokenizes the value and matches any token, this requires all tokens to
// appear in the given order. It renders to {"match_phrase": {field: {"query": value}}}.
type MatchPhraseQuery struct {
	field string
	value any
}

// NewMatchPhraseQuery creates a MatchPhraseQuery on the given field and value.
func NewMatchPhraseQuery(field string, value any) *MatchPhraseQuery {
	return &MatchPhraseQuery{field: field, value: value}
}

func (q *MatchPhraseQuery) Source() (any, error) {
	return map[string]any{
		"match_phrase": map[string]any{
			q.field: map[string]any{"query": q.value},
		},
	}, nil
}
