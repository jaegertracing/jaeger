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
	MaxChunkSize := 35 // max chunk size otel kafka export can handle safely.
	if td.SpanCount() > MaxChunkSize {
		w.logger.Info("Chunking traces")
		traceChunks := ChunkTraces(td, MaxChunkSize)
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
		newChunk     ptrace.Traces
		traceChunks  []ptrace.Traces
		currentSSpan ptrace.ScopeSpans
		currentRSpan ptrace.ResourceSpans
	)

	resources := td.ResourceSpans()
	currentChunkSize := 0
	newChunk = ptrace.NewTraces()

	for i := 0; i < resources.Len(); i++ {
		resource := resources.At(i)
		scopes := resource.ScopeSpans()

		if currentChunkSize > 0 {
			currentRSpan = newChunk.ResourceSpans().AppendEmpty()
			resource.Resource().Attributes().CopyTo(currentRSpan.Resource().Attributes())
		}
		for j := 0; j < scopes.Len(); j++ {
			scope := scopes.At(j)
			spans := scope.Spans()

			if currentChunkSize > 0 {
				currentSSpan = currentRSpan.ScopeSpans().AppendEmpty()
				scope.Scope().Attributes().CopyTo(currentSSpan.Scope().Attributes())
				currentSSpan.Scope().SetName(scope.Scope().Name())
				fmt.Println(scope.Scope().Name())
				currentSSpan.Scope().SetVersion(scope.Scope().Version())
			}
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				if currentChunkSize == 0 {
					currentRSpan = newChunk.ResourceSpans().AppendEmpty()
					resource.Resource().Attributes().CopyTo(currentRSpan.Resource().Attributes())
					currentSSpan = currentRSpan.ScopeSpans().AppendEmpty()
					scope.Scope().Attributes().CopyTo(currentSSpan.Scope().Attributes())
					currentSSpan.Scope().SetName(scope.Scope().Name())
					fmt.Println(scope.Scope().Name())
					currentSSpan.Scope().SetVersion(scope.Scope().Version())
				}
				span.CopyTo(currentSSpan.Spans().AppendEmpty())
				currentChunkSize++
				if currentChunkSize >= maxChunkSize {
					traceChunks = append(traceChunks, newChunk)
					newChunk = ptrace.NewTraces()
					currentChunkSize = 0
				}
			}
		}
	}

	if newChunk.ResourceSpans().Len() > 0 {
		traceChunks = append(traceChunks, newChunk)
	}

	return traceChunks
}
