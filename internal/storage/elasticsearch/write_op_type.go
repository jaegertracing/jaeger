// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

// WriteOpType is the Elasticsearch/OpenSearch bulk operation type.
type WriteOpType string

const (
	// WriteOpIndex is the standard "index" operation (upsert semantics).
	WriteOpIndex WriteOpType = "index"

	// WriteOpCreate is the "create" operation (fail if document exists).
	// Required by data streams, which are append-only.
	WriteOpCreate WriteOpType = "create"
)
