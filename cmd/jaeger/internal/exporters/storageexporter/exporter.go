// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storageexporter

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/internal/jptrace/sanitizer"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// TraceWriter is the write step behind jaeger_storage_exporter: at Start it resolves
// the configured trace storage from the jaeger_storage extension, and WriteTraces
// sanitizes each batch and writes it there. It is exported so that a component
// which needs the same write path, such as a connector that re-routes the spans the
// storage rejects, resolves and writes to storage exactly as the exporter does.
type TraceWriter struct {
	config    *Config
	logger    *zap.Logger
	storage   tracestore.Writer
	sanitizer sanitizer.Func
}

// NewTraceWriter returns a TraceWriter that resolves config.TraceStorage at Start.
func NewTraceWriter(config *Config, otel component.TelemetrySettings) *TraceWriter {
	return &TraceWriter{
		config:    config,
		logger:    otel.Logger,
		sanitizer: sanitizer.Sanitize,
	}
}

// Start resolves the trace storage named in the config from the host's
// jaeger_storage extension and creates its writer.
func (w *TraceWriter) Start(_ context.Context, host component.Host) error {
	f, err := jaegerstorage.GetTraceStoreFactory(w.config.TraceStorage, host)
	if err != nil {
		return fmt.Errorf("cannot find storage factory: %w", err)
	}

	w.warnMisalignedSyncBatchSizing(f)

	if w.storage, err = f.CreateTraceWriter(); err != nil {
		return fmt.Errorf("cannot create trace writer: %w", err)
	}

	return nil
}

// warnMisalignedSyncBatchSizing logs a warning when a synchronous, byte-capped
// storage (tracestore.SyncBulkWriteConfig) is paired with an exporter whose
// byte-sized queue.batch.max_size is unbounded or larger than half the writer's
// per-request cap. The two are measured differently: the batcher sizes a batch in
// OTLP protobuf bytes, while the writer chunks by the NDJSON _bulk body, which is
// 1.3x to 3.6x larger (hex ids repeated in the action line, the resource copied into
// every span document, decimal timestamps, repeated keys). Above half the cap the
// writer has to split most batches into several _bulk requests (RFC 0007 §4.4).
//
// This is only an efficiency concern, not a correctness one, so it is a warning
// rather than a startup error: if a split sub-request fails, the whole batch is
// retried, and because spans carry a deterministic _id (RFC 0007 §4.7) the re-sent
// sub-requests upsert rather than duplicate. Aligning the sizes just avoids the
// redundant re-writes and keeps one request per batch. The check applies only to a
// byte-sized batch, the only unit comparable to a byte cap; an item- or
// request-sized batch is skipped and left to documentation.
func (w *TraceWriter) warnMisalignedSyncBatchSizing(f tracestore.Factory) {
	sw, ok := f.(tracestore.SyncBulkWriteConfig)
	if !ok {
		return
	}
	sync, maxBytes := sw.SyncBulkWriteByteCap()
	if !sync || maxBytes <= 0 || !w.config.QueueConfig.HasValue() {
		return
	}
	queue := w.config.QueueConfig.Get()
	if !queue.Batch.HasValue() {
		return
	}
	batch := queue.Batch.Get()
	if batch.Sizer != exporterhelper.RequestSizerTypeBytes {
		return
	}
	if batch.MaxSize <= 0 || 2*batch.MaxSize > int64(maxBytes) {
		w.logger.Warn(
			"queue.batch.max_size is not aligned with the storage's bulk_processing.max_bytes; "+
				"with write_mode: sync the writer will split oversized batches into multiple _bulk "+
				"requests. Writes stay correct — retries are idempotent via the deterministic span _id "+
				"— but this is less efficient; the _bulk body is 1.3x to 3.6x the batch's protobuf "+
				"size, so set a byte-sized queue.batch.max_size at most half of max_bytes to keep one "+
				"request per batch (ADR-014)",
			zap.Int64("queue.batch.max_size", batch.MaxSize),
			zap.Int("bulk_processing.max_bytes", maxBytes),
		)
	}
}

func (*TraceWriter) close(_ context.Context) error {
	// span writer is not closable
	return nil
}

// WriteTraces sanitizes the batch and writes it to the resolved storage, returning
// the storage's error verbatim.
func (w *TraceWriter) WriteTraces(ctx context.Context, td ptrace.Traces) error {
	return w.storage.WriteTraces(ctx, w.sanitizer(td))
}
