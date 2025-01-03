// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"fmt"
	"io"
	"time"

	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/exporter/otlpexporter"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/storage_v2/tracestore"
)

var (
	_ tracestore.Writer = (*traceWriter)(nil)
	_ io.Closer         = (*traceWriter)(nil)

	MaxChunkSize = 35 // max chunk size otel kafka export can handle safely.
)

// traceWriter utilizes the OTLP exporter to send span data to the Jaeger-v2 receiver
type traceWriter struct {
	logger   *zap.Logger
	exporter exporter.Traces
}

func createTraceWriter(logger *zap.Logger, port int) (*traceWriter, error) {
	logger.Info("Creating the trace writer", zap.Int("port", port))

	factory := otlpexporter.NewFactory()
	cfg := factory.CreateDefaultConfig().(*otlpexporter.Config)
	cfg.Endpoint = fmt.Sprintf("localhost:%d", port)
	cfg.Timeout = 30 * time.Second
	cfg.RetryConfig.Enabled = false
	cfg.QueueConfig.Enabled = false
	cfg.TLSSetting = configtls.ClientConfig{
		Insecure: true,
	}

	set := exportertest.NewNopSettings()
	set.Logger = logger

	exp, err := factory.CreateTraces(context.Background(), set, cfg)
	if err != nil {
		return nil, err
	}
	if err := exp.Start(context.Background(), componenttest.NewNopHost()); err != nil {
		return nil, err
	}

	return &traceWriter{
		logger:   logger,
		exporter: exp,
	}, nil
}

func (w *traceWriter) Close() error {
	w.logger.Info("Closing the trace writer")
	return w.exporter.Shutdown(context.Background())
}

func (w *traceWriter) WriteTraces(ctx context.Context, td ptrace.Traces) error {
	var err error
	currentChunk := ptrace.NewTraces()
	currentResourceIndex := -1
	currentScopeIndex := -1
	spanCount := 0

	jptrace.SpanIter(td)(func(pos jptrace.SpanIterPos, span ptrace.Span) bool {
		var (
			scope    ptrace.ScopeSpans
			resource ptrace.ResourceSpans
		)

		if spanCount == MaxChunkSize {
			err = w.exporter.ConsumeTraces(ctx, currentChunk)
			currentChunk = ptrace.NewTraces()
			spanCount = 0
			currentResourceIndex = -1
			currentScopeIndex = -1
		}

		if currentResourceIndex != pos.ResourceIndex {
			resource = currentChunk.ResourceSpans().AppendEmpty()
			td.ResourceSpans().At(pos.ResourceIndex).Resource().Attributes().CopyTo(resource.Resource().Attributes())
			currentResourceIndex = pos.ResourceIndex
			currentScopeIndex = -1
		} else {
			resource = currentChunk.ResourceSpans().At(currentChunk.ResourceSpans().Len() - 1)
		}

		if currentScopeIndex != pos.ScopeIndex {
			scope = resource.ScopeSpans().AppendEmpty()
			td.ResourceSpans().At(pos.ResourceIndex).ScopeSpans().At(pos.ScopeIndex).Scope().CopyTo(scope.Scope())
			currentScopeIndex = pos.ScopeIndex
		} else {
			scope = resource.ScopeSpans().At(resource.ScopeSpans().Len() - 1)
		}

		span.CopyTo(scope.Spans().AppendEmpty())
		spanCount++

		return true
	})

	// write the last chunk if it has any spans
	if spanCount > 0 {
		err = w.exporter.ConsumeTraces(ctx, currentChunk)
	}
	return err
}
