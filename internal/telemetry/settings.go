// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/otel/metric"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/metrics/otelmetrics"
)

type Settings struct {
	Logger         *zap.Logger
	Metrics        metrics.Factory
	MeterProvider  metric.MeterProvider
	TracerProvider trace.TracerProvider
	Host           component.Host
}

// ReportStatus reports a component status event.
// If Host is set, it delegates to componentstatus.ReportStatus.
// Otherwise, it logs the status as an info message.
func (s Settings) ReportStatus(event *componentstatus.Event) {
	if s.Host != nil {
		componentstatus.ReportStatus(s.Host, event)
	} else if s.Logger != nil {
		s.Logger.Info("status", zap.Stringer("status", event.Status()))
	}
}

func NoopSettings() Settings {
	return Settings{
		Logger:         zap.NewNop(),
		Metrics:        metrics.NullFactory,
		MeterProvider:  noopmetric.NewMeterProvider(),
		TracerProvider: nooptrace.NewTracerProvider(),
		Host:           componenttest.NewNopHost(),
	}
}

func FromOtelComponent(telset component.TelemetrySettings, host component.Host) Settings {
	return Settings{
		Logger:         telset.Logger,
		Metrics:        otelmetrics.NewFactory(telset.MeterProvider),
		MeterProvider:  telset.MeterProvider,
		TracerProvider: telset.TracerProvider,
		Host:           host,
	}
}

func (s Settings) ToOtelComponent() component.TelemetrySettings {
	return component.TelemetrySettings{
		Logger:         s.Logger,
		MeterProvider:  s.MeterProvider,
		TracerProvider: s.TracerProvider,
	}
}
