// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// MaxAggregation computes the maximum value of a numeric field. It renders to
// {"max": {"field": field}}, matching what the storage layer previously produced
// via olivere's MaxAggregation. It is typically nested under a TermsAggregation
// to order buckets (e.g. latest startTime per traceID).
type MaxAggregation struct {
	field string
}

// NewMaxAggregation creates a MaxAggregation on the given field.
func NewMaxAggregation(field string) *MaxAggregation {
	return &MaxAggregation{field: field}
}

func (a *MaxAggregation) Source() (any, error) {
	return map[string]any{"max": map[string]any{"field": a.field}}, nil
}
