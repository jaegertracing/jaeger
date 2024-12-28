// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/jaegertracing/jaeger/storage_v2/tracestore"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/exporter/otlpexporter"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
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
	return w.exporter.ConsumeTraces(ctx, td)
}
