// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storageexporter

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/storage_v2/spanstore"
)

type storageExporter struct {
	config      *Config
	logger      *zap.Logger
	traceWriter spanstore.Writer
	clickhouse  bool
	// Separate traces exporting function for ClickHouse storage.
	// This is temporary until we have v2 storage API.
	chExportTraces func(ctx context.Context, td ptrace.Traces) error
}

func newExporter(config *Config, otel component.TelemetrySettings) *storageExporter {
	return &storageExporter{
		config: config,
		logger: otel.Logger,
	}
}

func (exp *storageExporter) start(_ context.Context, host component.Host) error {
	clickhouse, f, err := jaegerstorage.GetStorageFactoryV2(exp.config.TraceStorage, host)
	if err != nil {
		return fmt.Errorf("cannot find storage factory: %w", err)
	}

	if clickhouse {
		exp.clickhouse = clickhouse
		exp.chExportTraces = f.ChExportSpans
	} else {
		if exp.traceWriter, err = f.CreateTraceWriter(); err != nil {
			return fmt.Errorf("cannot create trace writer: %w", err)
		}
	}

	return nil
}

func (*storageExporter) close(_ context.Context) error {
	// span writer is not closable
	return nil
}

func (exp *storageExporter) pushTraces(ctx context.Context, td ptrace.Traces) error {
	if exp.clickhouse {
		return exp.chExportTraces(ctx, td)
	}

	return exp.traceWriter.WriteTraces(ctx, td)
}
