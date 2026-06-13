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
	queryName string
	params    map[string]any
}

// NewRangeQuery creates and initializes a new RangeQuery.
func NewRangeQuery(name string) *RangeQuery {
	return &RangeQuery{
		name:   name,
		params: make(map[string]any),
	}
}

// Generic setter
func (q *RangeQuery) set(key string, val any) *RangeQuery {
	q.params[key] = val
	return q
}

func (q *RangeQuery) Gt(val any) *RangeQuery      { return q.set("gt", val) }
func (q *RangeQuery) Gte(val any) *RangeQuery     { return q.set("gte", val) }
func (q *RangeQuery) Lt(val any) *RangeQuery      { return q.set("lt", val) }
func (q *RangeQuery) Lte(val any) *RangeQuery     { return q.set("lte", val) }
func (q *RangeQuery) Boost(b float64) *RangeQuery { return q.set("boost", b) }
func (q *RangeQuery) TimeZone(tz string) *RangeQuery {
	return q.set("time_zone", tz)
}

func (q *RangeQuery) Format(fmt string) *RangeQuery {
	return q.set("format", fmt)
}

func (q *RangeQuery) Relation(r string) *RangeQuery {
	return q.set("relation", r)
}

func (q *RangeQuery) QueryName(queryName string) *RangeQuery {
	q.queryName = queryName
	return q
}

// Source builds and returns the Elasticsearch-compatible representation of the range query.

func (q *RangeQuery) Source() (any, error) {
	source := make(map[string]any)
	rangeQ := make(map[string]any)
	source["range"] = rangeQ
	rangeQ[q.name] = q.params

	if q.queryName != "" {
		rangeQ["_name"] = q.queryName
	}
	return source, nil
}
