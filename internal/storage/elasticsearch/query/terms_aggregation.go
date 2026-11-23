// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// ExecutionHint selects the term-lookup strategy a terms aggregation uses.
// It renders to the "execution_hint" string Elasticsearch/OpenSearch expect.
type ExecutionHint string

const (
	// ExecutionHintGlobalOrdinals is the default: the aggregation builds global
	// ordinals for the field, which is fast for repeated aggregations over a
	// stable index but has to rebuild them whenever the index opens a new
	// reader.
	ExecutionHintGlobalOrdinals ExecutionHint = "global_ordinals"
	// ExecutionHintMap builds a term map instead of global ordinals. It costs
	// more per aggregation but never touches the global ordinals cache, so it
	// avoids the rebuild cost on indices that refresh often.
	ExecutionHintMap ExecutionHint = "map"
)

// TermsAggregation buckets documents by the distinct values of a field. It
// renders to {"terms": {"field": field, ...}} — optionally with a bucket "order"
// and sibling "aggregations" (sub-aggregations) — matching the shape the storage
// layer previously produced.
type TermsAggregation struct {
	field         string
	size          uint64
	order         []map[string]string
	executionHint ExecutionHint
	subAggs       map[string]Aggregation
}

// NewTermsAggregation creates a TermsAggregation on the given field.
func NewTermsAggregation(field string) *TermsAggregation {
	return &TermsAggregation{field: field}
}

// Size bounds the number of distinct buckets returned.
func (a *TermsAggregation) Size(size uint64) *TermsAggregation {
	a.size = size
	return a
}

// Order sorts buckets by the named metric (e.g. a sub-aggregation) in the given
// direction. Repeated calls append tie-breakers, and the whole set always renders
// as an array.
func (a *TermsAggregation) Order(name string, direction SortDirection) *TermsAggregation {
	a.order = append(a.order, map[string]string{name: string(direction)})
	return a
}

// ExecutionHint selects the term-lookup strategy for this aggregation. When
// unset the aggregation leaves the field unset, which lets Elasticsearch or
// OpenSearch apply its own default.
func (a *TermsAggregation) ExecutionHint(hint ExecutionHint) *TermsAggregation {
	a.executionHint = hint
	return a
}

// SubAggregation nests agg under this terms aggregation.
func (a *TermsAggregation) SubAggregation(name string, agg Aggregation) *TermsAggregation {
	if a.subAggs == nil {
		a.subAggs = make(map[string]Aggregation)
	}
	a.subAggs[name] = agg
	return a
}

func (a *TermsAggregation) Source() (any, error) {
	terms := map[string]any{"field": a.field}
	if a.size > 0 {
		terms["size"] = a.size
	}
	if len(a.order) > 0 {
		order := make([]any, len(a.order))
		for i, o := range a.order {
			order[i] = o
		}
		terms["order"] = order
	}
	if a.executionHint != "" {
		terms["execution_hint"] = string(a.executionHint)
	}
	result := map[string]any{"terms": terms}
	aggs, err := subAggregationsSource(a.subAggs)
	if err != nil {
		return nil, err
	}
	if aggs != nil {
		result["aggregations"] = aggs
	}
	return result, nil
}
