// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package telemetery

import (
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

type Setting struct {
	Logger         *zap.Logger
	TracerProvider trace.TracerProvider
	Metrics        metrics.Factory
	ReportStatus   func(*component.StatusEvent)
}

func HCAdapter(hc *healthcheck.HealthCheck) func(*component.StatusEvent) {
	return func(event *component.StatusEvent) {
		var hcStatus healthcheck.Status
		switch event.Status() {
		case component.StatusOK:
			hcStatus = healthcheck.Ready
		case component.StatusStarting,
			component.StatusRecoverableError,
			component.StatusPermanentError,
			component.StatusNone,
			component.StatusStopping,
			component.StatusStopped:
			hcStatus = healthcheck.Unavailable
		case component.StatusFatalError:
			hcStatus = healthcheck.Broken
		}
		hc.Set(hcStatus)
	}
}
