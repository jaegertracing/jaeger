// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storagewriterconnector

import "github.com/jaegertracing/jaeger/cmd/jaeger/internal/exporters/storageexporter"

// Config configures the jaeger_storage_writer connector. It is the
// jaeger_storage_exporter configuration (trace_storage, queue, retry_on_failure),
// because the connector is that exporter's write path with a dead-letter output
// bolted on: the same sending queue, blocking batcher, and retry policy apply to
// the storage write (RFC 0007 §4.2, §4.5).
//
// The named trace storage must report the spans it rejects terminally through a
// *esclient.BulkWriteError: for Elasticsearch/OpenSearch that is write_mode: sync
// with poison_pill_handling: fail. Against any other storage the connector writes
// like the exporter and nothing ever reaches the dead-letter pipeline.
type Config = storageexporter.Config
