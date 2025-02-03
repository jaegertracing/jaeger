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
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/sanitizer"
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
		config: config,
		logger: otel.Logger,
		sanitizer: sanitizer.NewChainedSanitizer(
			sanitizer.NewStandardSanitizers()...,
		),
	}
}

func (exp *storageExporter) start(_ context.Context, host component.Host) error {
	f, err := jaegerstorage.GetTraceStoreFactory(exp.config.TraceStorage, host)
	if err != nil {
		return fmt.Errorf("cannot find storage factory: %w", err)
	}

	if exp.traceWriter, err = f.CreateTraceWriter(); err != nil {
		return fmt.Errorf("cannot create trace writer: %w", err)
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
