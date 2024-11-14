// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package telemetery

import (
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/config/configtelemetry"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

type Setting struct {
	Logger               *zap.Logger
	Metrics              metrics.Factory
	LeveledMeterProvider func(configtelemetry.Level) metric.MeterProvider
	TracerProvider       trace.TracerProvider
	ReportStatus         func(*componentstatus.Event)
	Host                 component.Host
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
