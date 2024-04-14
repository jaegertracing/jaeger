// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"fmt"
	"io"

	jaeger2otlp "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/exporter/otlpexporter"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var (
	_ spanstore.Writer = (*spanWriter)(nil)
	_ io.Closer        = (*spanWriter)(nil)
)

// SpanWriter utilizes the OTLP exporter to send span data to the Jaeger-v2 receiver
type spanWriter struct {
	exporter exporter.Traces
}

func createSpanWriter(logger *zap.Logger, port int) (*spanWriter, error) {
	factory := otlpexporter.NewFactory()
	cfg := factory.CreateDefaultConfig().(*otlpexporter.Config)
	cfg.Endpoint = fmt.Sprintf("localhost:%d", port)
	cfg.RetryConfig.Enabled = false
	cfg.QueueConfig.Enabled = false
	cfg.TLSSetting = configtls.ClientConfig{
		Insecure: true,
	}

	set := exportertest.NewNopCreateSettings()
	set.Logger = logger

	exporter, err := factory.CreateTracesExporter(context.Background(), set, cfg)
	if err != nil {
		return nil, err
	}
	if err := exporter.Start(context.Background(), componenttest.NewNopHost()); err != nil {
		return nil, err
	}

	return &spanWriter{
		exporter: exporter,
	}, nil
}

func (w *spanWriter) Close() error {
	return w.exporter.Shutdown(context.Background())
}

func (w *spanWriter) WriteSpan(ctx context.Context, span *model.Span) error {
	td, err := jaeger2otlp.ProtoToTraces([]*model.Batch{
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
