// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package query builds the Elasticsearch/OpenSearch query and aggregation JSON
// that the storage layer sends — the request-body AST, each node rendering its
// own wire form via Source().
package query
