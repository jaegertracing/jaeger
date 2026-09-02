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

type storageExporter struct {
	config      *Config
	logger      *zap.Logger
	traceWriter tracestore.Writer
	sanitizer   sanitizer.Func
}

func newExporter(config *Config, otel component.TelemetrySettings) *storageExporter {
	return &storageExporter{
		config:    config,
		logger:    otel.Logger,
		sanitizer: sanitizer.Sanitize,
	}
}

func (exp *storageExporter) start(_ context.Context, host component.Host) error {
	f, err := jaegerstorage.GetTraceStoreFactory(exp.config.TraceStorage, host)
	if err != nil {
		return fmt.Errorf("cannot find storage factory: %w", err)
	}

	exp.warnMisalignedSyncBatchSizing(f)

	if exp.traceWriter, err = f.CreateTraceWriter(); err != nil {
		return fmt.Errorf("cannot create trace writer: %w", err)
	}

	return nil
}

// warnMisalignedSyncBatchSizing logs a warning when a synchronous, byte-capped
// storage (tracestore.SyncBulkWriteConfig) is paired with an exporter whose
// byte-sized queue.batch.max_size is unbounded or larger than the writer's
// per-request cap. The writer then has to split an oversized batch into several
// _bulk requests (RFC 0007 §4.4).
//
// This is only an efficiency concern, not a correctness one, so it is a warning
// rather than a startup error: if a split sub-request fails, the whole batch is
// retried, and because spans carry a deterministic _id (RFC 0007 §4.7) the re-sent
// sub-requests upsert rather than duplicate. Aligning the sizes just avoids the
// redundant re-writes and keeps one request per batch. The check applies only to a
// byte-sized batch, the only unit comparable to a byte cap; an item- or
// request-sized batch is skipped and left to documentation.
func (exp *storageExporter) warnMisalignedSyncBatchSizing(f tracestore.Factory) {
	sw, ok := f.(tracestore.SyncBulkWriteConfig)
	if !ok {
		return
	}
	sync, maxBytes := sw.SyncBulkWriteByteCap()
	if !sync || maxBytes <= 0 || !exp.config.QueueConfig.HasValue() {
		return
	}
	queue := exp.config.QueueConfig.Get()
	if !queue.Batch.HasValue() {
		return
	}
	batch := queue.Batch.Get()
	if batch.Sizer != exporterhelper.RequestSizerTypeBytes {
		return
	}
	if batch.MaxSize <= 0 || batch.MaxSize > int64(maxBytes) {
		exp.logger.Warn(
			"queue.batch.max_size is not aligned with the storage's bulk_processing.max_bytes; "+
				"with write_mode: sync the writer will split oversized batches into multiple _bulk "+
				"requests. Writes stay correct — retries are idempotent via the deterministic span _id "+
				"— but this is less efficient; set a byte-sized queue.batch.max_size no larger than "+
				"max_bytes to keep one request per batch (RFC 0007 §4.5)",
			zap.Int64("queue.batch.max_size", batch.MaxSize),
			zap.Int("bulk_processing.max_bytes", maxBytes),
		)
	}
}

func (*storageExporter) close(_ context.Context) error {
	// span writer is not closable
	return nil
}

func (exp *storageExporter) pushTraces(ctx context.Context, td ptrace.Traces) error {
	return exp.traceWriter.WriteTraces(ctx, exp.sanitizer(td))
}
