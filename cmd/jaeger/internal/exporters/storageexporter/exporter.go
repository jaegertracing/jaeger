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

// syncBulkWriteConfig is implemented by trace-store factories that persist each
// batch as a single, byte-capped synchronous request (Elasticsearch/OpenSearch
// write_mode: sync). The exporter uses it to validate, before serving traffic,
// that its own sending queue cannot hand the writer a batch larger than that
// per-request cap. Backends that do not implement it — or are not in synchronous
// mode — skip the check.
type syncBulkWriteConfig interface {
	// SyncBulkWriteByteCap reports whether writes are synchronous and, if so, the
	// maximum number of bytes the writer puts in a single request.
	SyncBulkWriteByteCap() (sync bool, maxBytes int)
}

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

	if err := exp.validateSyncBatchSizing(f); err != nil {
		return err
	}

	if exp.traceWriter, err = f.CreateTraceWriter(); err != nil {
		return fmt.Errorf("cannot create trace writer: %w", err)
	}

	return nil
}

// validateSyncBatchSizing hard-fails startup when the storage writes each batch as
// one byte-capped synchronous request (write_mode: sync) and this exporter's
// sending queue could hand it a batch that does not fit that cap. An oversized
// batch would be split into several requests, so it would no longer be atomic: a
// partial failure could persist some spans while the source (e.g. a Kafka offset)
// is not advanced for the rest — defeating the at-least-once guarantee synchronous
// mode exists to provide (RFC 0007 §4.5).
//
// The check applies only to a byte-sized batch, the only unit comparable to a byte
// cap; an item- or request-sized batch is skipped and left to documentation. A
// byte-sized batch must be bounded (max_size > 0) and within the cap: an unbounded
// batch is the most dangerous case, as it can grow arbitrarily past the cap.
func (exp *storageExporter) validateSyncBatchSizing(f tracestore.Factory) error {
	sw, ok := f.(syncBulkWriteConfig)
	if !ok {
		return nil
	}
	sync, maxBytes := sw.SyncBulkWriteByteCap()
	if !sync || maxBytes <= 0 || !exp.config.QueueConfig.HasValue() {
		return nil
	}
	queue := exp.config.QueueConfig.Get()
	if !queue.Batch.HasValue() {
		return nil
	}
	batch := queue.Batch.Get()
	if batch.Sizer != exporterhelper.RequestSizerTypeBytes {
		return nil
	}
	if batch.MaxSize <= 0 || batch.MaxSize > int64(maxBytes) {
		return fmt.Errorf(
			"invalid jaeger_storage_exporter config: with write_mode: sync the byte-sized "+
				"queue.batch.max_size must be a positive value not exceeding the storage's "+
				"bulk_processing.max_bytes (%d), so each batch is one atomic request "+
				"(RFC 0007 §4.5); got max_size=%d",
			maxBytes, batch.MaxSize,
		)
	}
	return nil
}

func (*storageExporter) close(_ context.Context) error {
	// span writer is not closable
	return nil
}

func (exp *storageExporter) pushTraces(ctx context.Context, td ptrace.Traces) error {
	return exp.traceWriter.WriteTraces(ctx, exp.sanitizer(td))
}
