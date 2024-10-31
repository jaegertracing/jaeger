// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package telemetery

import (
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

type Setting struct {
	Logger         *zap.Logger
	TracerProvider trace.TracerProvider
	Metrics        metrics.Factory
	ReportStatus   func(*componentstatus.Event)
	Host           component.Host
	MeterProvider  metric.MeterProvider
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

func InitializeNewMeterProvider() *Setting {
	meterProvider := metric.NewMeterProvider()
	return &Setting{
		MeterProvider: meterProvider,
	}
}
