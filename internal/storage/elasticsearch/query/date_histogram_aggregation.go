// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// DateHistogramAggregation buckets documents into fixed-width time intervals. It
// renders to {"date_histogram": {"field": …, "fixed_interval": …, "min_doc_count":
// …, "extended_bounds": {"min": …, "max": …}}} with an optional sibling
// "aggregations", matching what the storage layer previously produced via
// olivere's DateHistogramAggregation. It backs the metricstore time series.
type DateHistogramAggregation struct {
	field            string
	fixedInterval    string
	minDocCount      int
	minDocCountSet   bool
	extendedMin      int64
	extendedMax      int64
	extendedBoundsOn bool
	subAggs          map[string]Aggregation
}

// NewDateHistogramAggregation creates an empty DateHistogramAggregation.
func NewDateHistogramAggregation() *DateHistogramAggregation {
	return &DateHistogramAggregation{}
}

// Field sets the date field to bucket on.
func (a *DateHistogramAggregation) Field(field string) *DateHistogramAggregation {
	a.field = field
	return a
}

// FixedInterval sets the bucket width (e.g. "60000ms").
func (a *DateHistogramAggregation) FixedInterval(interval string) *DateHistogramAggregation {
	a.fixedInterval = interval
	return a
}

// MinDocCount sets the minimum document count for a bucket to be returned; 0
// keeps empty buckets so the time series has a point per interval.
func (a *DateHistogramAggregation) MinDocCount(minDocCount int) *DateHistogramAggregation {
	a.minDocCount = minDocCount
	a.minDocCountSet = true
	return a
}

// ExtendedBounds forces the histogram to span [min, max] even where there are no
// documents, so the series covers the whole requested range.
func (a *DateHistogramAggregation) ExtendedBounds(minMillis, maxMillis int64) *DateHistogramAggregation {
	a.extendedMin = minMillis
	a.extendedMax = maxMillis
	a.extendedBoundsOn = true
	return a
}

// SubAggregation nests agg under each histogram bucket.
func (a *DateHistogramAggregation) SubAggregation(name string, agg Aggregation) *DateHistogramAggregation {
	if a.subAggs == nil {
		a.subAggs = make(map[string]Aggregation)
	}
	a.subAggs[name] = agg
	return a
}

func (a *DateHistogramAggregation) Source() (any, error) {
	dateHist := map[string]any{}
	if a.field != "" {
		dateHist["field"] = a.field
	}
	if a.fixedInterval != "" {
		dateHist["fixed_interval"] = a.fixedInterval
	}
	if a.minDocCountSet {
		dateHist["min_doc_count"] = a.minDocCount
	}
	if a.extendedBoundsOn {
		dateHist["extended_bounds"] = map[string]any{
			"min": a.extendedMin,
			"max": a.extendedMax,
		}
	}
	result := map[string]any{"date_histogram": dateHist}
	aggs, err := subAggregationsSource(a.subAggs)
	if err != nil {
		return nil, err
	}
	if aggs != nil {
		result["aggregations"] = aggs
	}
	return result, nil
}
