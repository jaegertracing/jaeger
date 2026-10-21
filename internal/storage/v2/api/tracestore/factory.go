// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

// Factory defines an interface for a factory that can create implementations of
// different span storage components.
type Factory interface {
	// CreateTraceReader creates a spanstore.Reader.
	CreateTraceReader() (Reader, error)

	// CreateTraceWriter creates a spanstore.Writer.
	CreateTraceWriter() (Writer, error)
}

// SyncBulkWriteConfig is an optional capability of a Factory whose writer persists
// each batch as a single, byte-capped request (e.g. Elasticsearch/OpenSearch
// write_mode: sync). Callers that size their batches against the writer's
// per-request cap type-assert a Factory to this interface; factories that do not
// implement it, or are not in that mode, are skipped.
type SyncBulkWriteConfig interface {
	// SyncBulkWriteByteCap reports whether writes are synchronous and, if so, the
	// maximum number of bytes the writer puts in a single request.
	SyncBulkWriteByteCap() (sync bool, maxBytes int)
}
