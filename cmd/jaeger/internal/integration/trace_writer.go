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
	maxChunkSize := 35 // max chunk size otel kafka export can handle safely.
	if td.SpanCount() > maxChunkSize {
		w.logger.Info("Chunking traces")
		traceChunks := ChunkTraces(td, maxChunkSize)
		for _, chunk := range traceChunks {
			err = w.exporter.ConsumeTraces(ctx, chunk)
			if err != nil {
				return err
			}
		}
		w.logger.Info("Finished writing chunks", zap.Int("number of chunks written", len(traceChunks)))
	} else {
		err = w.exporter.ConsumeTraces(ctx, td)
	}
	return err
}

// ChunkTraces splits td into chunks of ptrace.Traces.
// Each chunk has a span count equal to maxCHunkSize
//
// maxChunkSize should always be greater than or equal to 2
func ChunkTraces(td ptrace.Traces, maxChunkSize int) []ptrace.Traces {
	var (
		traceChunks          []ptrace.Traces
		currentChunk         ptrace.Traces
		spanCount            int
		currentResourceIndex int
		currentScopeIndex    int
	)

	currentChunk = ptrace.NewTraces()

	jptrace.SpanIter(td)(func(pos jptrace.SpanIterPos, span ptrace.Span) bool {
		var scope ptrace.ScopeSpans
		var resource ptrace.ResourceSpans
		// Create a new chunk if the current span count reaches maxChunkSize
		if spanCount == maxChunkSize {
			traceChunks = append(traceChunks, currentChunk)
			currentChunk = ptrace.NewTraces()
			spanCount = 0
		}

		if currentChunk.ResourceSpans().Len() == 0 || currentResourceIndex != pos.ResourceIndex {
			resource = currentChunk.ResourceSpans().AppendEmpty()
			td.ResourceSpans().At(pos.ResourceIndex).Resource().Attributes().CopyTo(resource.Resource().Attributes())
			currentResourceIndex = pos.ResourceIndex
		} else {
			resource = currentChunk.ResourceSpans().At(currentChunk.ResourceSpans().Len() - 1)
		}

		if resource.ScopeSpans().Len() == 0 || currentScopeIndex != pos.ScopeIndex {
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

	// append the last chunk if it has any spans
	if spanCount > 0 {
		traceChunks = append(traceChunks, currentChunk)
	}

	return traceChunks
}
