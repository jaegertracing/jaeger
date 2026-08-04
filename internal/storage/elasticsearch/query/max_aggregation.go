// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// MaxAggregation computes the maximum of either a numeric field or a Painless
// script. It renders to {"max": {"field": field}} or {"max": {"script":
// {"source": script}}}, matching what the storage layer previously produced. It
// is typically nested under a TermsAggregation to
// order buckets (e.g. latest startTime per traceID).
type MaxAggregation struct {
	field  string
	script string
}

// NewMaxAggregation creates a MaxAggregation over the given field.
func NewMaxAggregation(field string) *MaxAggregation {
	return &MaxAggregation{field: field}
}

// NewScriptedMaxAggregation creates a MaxAggregation over a Painless script
// (used where the value is derived, e.g. end time = startTime + duration).
func NewScriptedMaxAggregation(script string) *MaxAggregation {
	return &MaxAggregation{script: script}
}

func (a *MaxAggregation) Source() (any, error) {
	inner := map[string]any{}
	if a.script != "" {
		inner["script"] = map[string]any{"source": a.script}
	} else {
		inner["field"] = a.field
	}
	return map[string]any{"max": inner}, nil
}
