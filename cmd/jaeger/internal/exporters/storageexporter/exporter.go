// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storageexporter

import (
	"context"
	"errors"
	"fmt"

	otlp2jaeger "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

type storageExporter struct {
	config     *Config
	logger     *zap.Logger
	spanWriter spanstore.Writer
}

func newExporter(config *Config, otel component.TelemetrySettings) *storageExporter {
	return &storageExporter{
		config: config,
		logger: otel.Logger,
	}
}

func (exp *storageExporter) start(_ context.Context, host component.Host) error {
	f, err := jaegerstorage.GetStorageFactory(exp.config.TraceStorage, host)
	if err != nil {
		return fmt.Errorf("cannot find storage factory: %w", err)
	}

	if exp.spanWriter, err = f.CreateSpanWriter(); err != nil {
		return fmt.Errorf("cannot create span writer: %w", err)
	}

	return nil
}

func (exp *storageExporter) close(_ context.Context) error {
	// span writer is not closable
	return nil
}

func (exp *storageExporter) pushTraces(ctx context.Context, td ptrace.Traces) error {
	batches, err := otlp2jaeger.ProtoFromTraces(td)
	if err != nil {
		return fmt.Errorf("cannot transform OTLP traces to Jaeger format: %w", err)
	}
	var errs []error
	for _, batch := range batches {
		for _, span := range batch.Spans {
			if span.Process == nil {
				span.Process = batch.Process
			}
			errs = append(errs, exp.spanWriter.WriteSpan(ctx, span))
		}
	}
	return errors.Join(errs...)
}
