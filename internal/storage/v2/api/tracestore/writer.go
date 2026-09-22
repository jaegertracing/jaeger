// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Writer writes spans to storage.
type Writer interface {
	// WriteTraces writes a batch of spans to storage. Idempotent.
	// Implementations are not required to support atomic transactions,
	// so if any of the spans fail to be written an error is returned.
	// Compatible with OTLP Exporter API.
	//
	// A conformant implementation writes synchronously: it returns nil only
	// after the batch is durably persisted, and returns a real error otherwise,
	// so a caller (e.g. the Kafka ingester) can retry or apply backpressure
	// rather than silently dropping data. Returning nil before the write is
	// durable — i.e. asynchronous, fire-and-forget writing — is a deliberate
	// deviation that trades this guarantee for throughput on unbatched direct
	// ingest, and must be an explicit, documented mode rather than the default
	// (see RFC 0007). The Cassandra and ClickHouse writers are synchronous; the
	// Elasticsearch/OpenSearch writer is asynchronous by default (RFC 0007
	// introduces an opt-in synchronous mode).
	WriteTraces(ctx context.Context, td ptrace.Traces) error
}

// RejectedSpan identifies one span the backend rejected terminally, with the
// backend's reason.
type RejectedSpan struct {
	TraceID pcommon.TraceID
	SpanID  pcommon.SpanID
	Reason  string
}

// RejectedSpansError is the error a synchronous Writer returns when the backend
// stored a batch except for spans it rejected terminally: poison pills that would
// fail identically on every retry (a mapping conflict, a malformed field). It lets a
// caller that can re-route spans, such as a dead-letter connector, recover exactly
// which spans need re-routing with errors.As, without knowing anything about the
// backend's document layout (RFC 0007 §4.8). A Writer that does not distinguish
// terminal from transient failures never returns it.
//
// The batch is complete, and the caller may acknowledge it once Spans are
// re-routed, only when Transient is false and Unidentified is zero.
type RejectedSpansError struct {
	// Spans are the terminally-rejected spans.
	Spans []RejectedSpan
	// Transient reports that the batch also had failures worth retrying (a
	// backend under pressure, a transport failure), so the whole batch must be
	// retried; a Writer with idempotent writes stores nothing twice on the retry.
	Transient bool
	// Unidentified counts rejected documents the Writer could not attribute to a
	// span. Any of them could be a span, so the batch must not be acknowledged.
	Unidentified int
	// Err is the backend's own error, kept for its message; it may be nil.
	Err error
}

func (e *RejectedSpansError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("%d spans rejected by the storage", len(e.Spans))
	}
	return e.Err.Error()
}

func (e *RejectedSpansError) Unwrap() error {
	return e.Err
}
