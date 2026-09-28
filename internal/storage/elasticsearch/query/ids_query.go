// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// IdsQuery matches documents by their _id. It renders to
// {"ids": {"values": [ids...]}}, and unlike a term query on _id it needs no
// fielddata on that field, which Elasticsearch 8 and later disable by default.
type IdsQuery struct {
	values []string
}

// NewIdsQuery creates an IdsQuery matching any of the given document ids.
func NewIdsQuery(ids ...string) *IdsQuery {
	return &IdsQuery{values: ids}
}

func (q *IdsQuery) Source() (any, error) {
	return map[string]any{"ids": map[string]any{"values": q.values}}, nil
}
