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

// PoisonPillReporting is an optional capability of a Factory whose writer, instead
// of failing or silently dropping a batch that the backend rejected in part, reports
// the terminally-rejected spans to its caller (e.g. Elasticsearch/OpenSearch
// write_mode: sync with poison_pill_handling: fail, whose WriteTraces returns a
// *esclient.BulkWriteError listing them). A component that dead-letters those
// spans onto another pipeline type-asserts a Factory to this interface at startup
// to confirm the backend will ever hand it anything to dead-letter (RFC 0007 §4.8).
type PoisonPillReporting interface {
	// ReportsPoisonPills reports whether WriteTraces surfaces terminally-rejected
	// spans to the caller rather than dropping them or retrying them forever.
	ReportsPoisonPills() bool
}
