// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// Package query provides an Elasticsearch RangeQuery implementation.
// This RangeQuery behaves the same as the Go Elasticsearch client (olivere/elastic),
// but is rewritten to be compatible with Elasticsearch v9 and avoids deprecated parameters.
//
// Deprecated parameters like include_lower, include_upper, from, and to are excluded deliberately.

type RangeQuery struct {
	name      string
	gt        any
	gte       any
	lt        any
	lte       any
	timeZone  string
	boost     *float64
	queryName string
	format    string
	relation  string
}

// NewRangeQuery creates and initializes a new RangeQuery.
func NewRangeQuery(name string) *RangeQuery {
	return &RangeQuery{name: name}
}

func (q *RangeQuery) Gt(val any) *RangeQuery {
	q.gt = val
	return q
}

func (q *RangeQuery) Gte(val any) *RangeQuery {
	q.gte = val
	return q
}

func (q *RangeQuery) Lt(val any) *RangeQuery {
	q.lt = val
	return q
}

func (q *RangeQuery) Lte(val any) *RangeQuery {
	q.lte = val
	return q
}

func (q *RangeQuery) Boost(boost float64) *RangeQuery {
	q.boost = &boost
	return q
}

func (q *RangeQuery) QueryName(queryName string) *RangeQuery {
	q.queryName = queryName
	return q
}

func (q *RangeQuery) TimeZone(timeZone string) *RangeQuery {
	q.timeZone = timeZone
	return q
}

func (q *RangeQuery) Format(format string) *RangeQuery {
	q.format = format
	return q
}

func (q *RangeQuery) Relation(relation string) *RangeQuery {
	q.relation = relation
	return q
}

// Source builds and returns the Elasticsearch-compatible representation of the range query.

func (q *RangeQuery) Source() (any, error) {
	source := make(map[string]any)
	rangeQ := make(map[string]any)
	source["range"] = rangeQ
	params := make(map[string]any)
	rangeQ[q.name] = params

	if q.gt != nil {
		params["gt"] = q.gt
	}
	if q.gte != nil {
		params["gte"] = q.gte
	}
	if q.lt != nil {
		params["lt"] = q.lt
	}
	if q.lte != nil {
		params["lte"] = q.lte
	}
	if q.timeZone != "" {
		params["time_zone"] = q.timeZone
	}
	if q.format != "" {
		params["format"] = q.format
	}
	if q.relation != "" {
		params["relation"] = q.relation
	}
	if q.boost != nil {
		params["boost"] = *q.boost
	}
	if q.queryName != "" {
		rangeQ["_name"] = q.queryName
	}

	return source, nil
}
