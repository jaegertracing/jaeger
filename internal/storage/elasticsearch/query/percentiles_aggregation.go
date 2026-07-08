// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// PercentilesAggregation computes percentiles of a numeric field. It renders to
// {"percentiles": {"field": …, "percents": [...]}}, matching what the storage
// layer previously produced via olivere's PercentilesAggregation. It backs the
// metricstore latency quantiles.
type PercentilesAggregation struct {
	field    string
	percents []float64
}

// NewPercentilesAggregation creates an empty PercentilesAggregation.
func NewPercentilesAggregation() *PercentilesAggregation {
	return &PercentilesAggregation{}
}

// Field sets the numeric field to compute percentiles over.
func (a *PercentilesAggregation) Field(field string) *PercentilesAggregation {
	a.field = field
	return a
}

// Percentiles sets the percentile ranks to compute (e.g. 95 for p95).
func (a *PercentilesAggregation) Percentiles(percents ...float64) *PercentilesAggregation {
	a.percents = append(a.percents, percents...)
	return a
}

func (a *PercentilesAggregation) Source() (any, error) {
	percentiles := map[string]any{}
	if a.field != "" {
		percentiles["field"] = a.field
	}
	if len(a.percents) > 0 {
		percentiles["percents"] = a.percents
	}
	return map[string]any{"percentiles": percentiles}, nil
}
