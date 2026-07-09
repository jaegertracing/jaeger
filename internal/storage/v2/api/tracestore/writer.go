// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"

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
