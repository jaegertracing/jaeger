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
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var (
	_ spanstore.Writer = (*spanWriter)(nil)
	_ io.Closer        = (*spanWriter)(nil)
)

// SpanWriter utilizes the OTLP exporter to send span data to the Jaeger-v2 receiver
type spanWriter struct {
	logger   *zap.Logger
	exporter exporter.Traces
}

func createSpanWriter(logger *zap.Logger, port int) (*spanWriter, error) {
	logger.Info("Creating the span writer", zap.Int("port", port))

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

	return &spanWriter{
		logger:   logger,
		exporter: exp,
	}, nil
}

func (w *spanWriter) Close() error {
	w.logger.Info("Closing the span writer")
	return w.exporter.Shutdown(context.Background())
}

func (w *spanWriter) WriteSpan(ctx context.Context, span *model.Span) error {
	td, err := jptrace.ProtoToTraces([]*model.Batch{
		{
			Spans:   []*model.Span{span},
			Process: span.Process,
		},
	})
	if err != nil {
		return err
	}

	return w.exporter.ConsumeTraces(ctx, td)
}
