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

	"github.com/jaegertracing/jaeger/internal/metrics/otelmetrics"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

type Settings struct {
	Logger         *zap.Logger
	Metrics        metrics.Factory
	MeterProvider  metric.MeterProvider
	TracerProvider trace.TracerProvider
	ReportStatus   func(*componentstatus.Event) // TODO remove this
	Host           component.Host
}

func HCAdapter(hc *healthcheck.HealthCheck) func(*componentstatus.Event) {
	return func(event *componentstatus.Event) {
		var hcStatus healthcheck.Status
		switch event.Status() {
		case componentstatus.StatusOK:
			hcStatus = healthcheck.Ready
		case componentstatus.StatusStarting,
			componentstatus.StatusRecoverableError,
			componentstatus.StatusPermanentError,
			componentstatus.StatusNone,
			componentstatus.StatusStopping,
			componentstatus.StatusStopped:
			hcStatus = healthcheck.Unavailable
		case componentstatus.StatusFatalError:
			hcStatus = healthcheck.Broken
		}
		hc.Set(hcStatus)
	}
}

func NoopSettings() Settings {
	return Settings{
		Logger:         zap.NewNop(),
		Metrics:        metrics.NullFactory,
		MeterProvider:  noopmetric.NewMeterProvider(),
		TracerProvider: nooptrace.NewTracerProvider(),
		ReportStatus:   func(*componentstatus.Event) {},
		Host:           componenttest.NewNopHost(),
	}
}

func FromOtelComponent(telset component.TelemetrySettings, host component.Host) Settings {
	return Settings{
		Logger:         telset.Logger,
		Metrics:        otelmetrics.NewFactory(telset.MeterProvider),
		MeterProvider:  telset.MeterProvider,
		TracerProvider: telset.TracerProvider,
		ReportStatus: func(event *componentstatus.Event) {
			componentstatus.ReportStatus(host, event)
		},
		Host: host,
	}
}
