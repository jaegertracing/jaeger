// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// MinAggregation computes the minimum value of a numeric field. It renders to
// {"min": {"field": field}}, matching what the storage layer previously produced.
type MinAggregation struct {
	field string
}

// NewMinAggregation creates a MinAggregation on the given field.
func NewMinAggregation(field string) *MinAggregation {
	return &MinAggregation{field: field}
}

func (a *MinAggregation) Source() (any, error) {
	return map[string]any{"min": map[string]any{"field": a.field}}, nil
}
