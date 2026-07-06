// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// TermsAggregation buckets documents by the distinct values of a field. It
// renders to {"terms": {"field": field, "size": size}} — the shape the storage
// layer previously produced via olivere's TermsAggregation.
type TermsAggregation struct {
	field string
	size  int
}

// NewTermsAggregation creates a TermsAggregation on the given field.
func NewTermsAggregation(field string) *TermsAggregation {
	return &TermsAggregation{field: field}
}

// Size bounds the number of distinct buckets returned.
func (a *TermsAggregation) Size(size int) *TermsAggregation {
	a.size = size
	return a
}

func (a *TermsAggregation) Source() (any, error) {
	terms := map[string]any{"field": a.field}
	if a.size > 0 {
		terms["size"] = a.size
	}
	return map[string]any{"terms": terms}, nil
}
