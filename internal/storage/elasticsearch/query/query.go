// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// Query is an Elasticsearch/OpenSearch query node. Source renders it to the
// wire JSON representation (a value json.Marshal turns into the request body),
// so the storage layer builds queries from these owned nodes rather than a
// driver's query types.
type Query interface {
	Source() (any, error)
}

// Aggregation is an ES/OS aggregation node, rendered the same way as Query.
type Aggregation interface {
	Source() (any, error)
}
