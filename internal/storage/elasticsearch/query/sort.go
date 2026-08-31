// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

// SortDirection is the direction of a sort or bucket ordering. It renders to the
// "asc"/"desc" strings Elasticsearch/OpenSearch expect.
type SortDirection string

const (
	Ascending  SortDirection = "asc"
	Descending SortDirection = "desc"
)
