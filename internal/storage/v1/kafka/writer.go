// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package kafka

import (
	"context"
	"errors"

	kafkaexporter "github.com/open-telemetry/opentelemetry-collector-contrib/exporter/kafkaexporter"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
)

type spanWriterMetrics struct {
	SpansWrittenSuccess metrics.Counter
	SpansWrittenFailure metrics.Counter
}

// SpanWriter writes spans to kafka. Implements spanstore.Writer
type SpanWriter struct {
	metrics    spanWriterMetrics
	exporter   component.Component
	marshaller Marshaller
	pushFunc   func(ctx context.Context, td ptrace.Traces) error
}

// NewSpanWriter initiates and returns a new kafka spanwriter
func NewSpanWriter(
	topic string,
	factory metrics.Factory,
	brokers []string,
	marshaller Marshaller,
	logger *zap.Logger,
	telemetry component.TelemetrySettings,
) (*SpanWriter, error) {
	writeMetrics := spanWriterMetrics{
		SpansWrittenSuccess: factory.Counter(metrics.Options{Name: "kafka_spans_written", Tags: map[string]string{"status": "success"}}),
		SpansWrittenFailure: factory.Counter(metrics.Options{Name: "kafka_spans_written", Tags: map[string]string{"status": "failure"}}),
	}

	exportercfg := kafkaexporter.NewFactory().CreateDefaultConfig().(*kafkaexporter.Config)
	exportercfg.Topic = topic
	exportercfg.Brokers = brokers

	settings := exporter.Settings{
		ID: component.MustNewID("kafka"),
		TelemetrySettings: component.TelemetrySettings{
			Logger:         logger,
			TracerProvider: telemetry.TracerProvider,
			MeterProvider:  telemetry.MeterProvider,
		},
		BuildInfo: component.NewDefaultBuildInfo(),
	}

	exp, err := kafkaexporter.NewFactory().CreateTraces(context.Background(), settings, exportercfg)
	if err != nil {
		return nil, err
	}

	return &SpanWriter{
		metrics:    writeMetrics,
		exporter:   exp,
		marshaller: marshaller,
		pushFunc:   exp.ConsumeTraces,
	}, nil
}

// WriteSpan writes the span to kafka.
func (w *SpanWriter) WriteSpan(ctx context.Context, span *model.Span) error {
	batch := &model.Batch{Spans: []*model.Span{span}}
	traces := v1adapter.V1BatchesToTraces([]*model.Batch{batch})
	if traces.SpanCount() == 0 {
		w.metrics.SpansWrittenFailure.Inc(1)
		return errors.New("trace data conversion resulted in empty trace")
	}

	err := w.pushFunc(ctx, traces)
	if err != nil {
		w.metrics.SpansWrittenFailure.Inc(1)
		return err
	}
	w.metrics.SpansWrittenSuccess.Inc(1)
	// The AsyncProducer accepts messages on a channel and produces them asynchronously
	// in the background as efficiently as possible
	return nil
}

// Close closes SpanWriter by closing producer
func (w *SpanWriter) Close() error {
	if w.exporter != nil {
		return w.exporter.Shutdown(context.Background())
	}
	return nil
}
