// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package telemetery

import (
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/jtracer"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"go.opentelemetry.io/collector/component"
)

type Setting struct {
	Logger       *zap.Logger
	Tracer       *jtracer.JTracer
	Metrics      metrics.Factory
	ReportStatus func(*component.StatusEvent)
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
			component.StatusStopping:
			hcStatus = healthcheck.Unavailable
		case component.StatusFatalError, component.StatusStopped:
			hcStatus = healthcheck.Broken
		}
		hc.Set(hcStatus)
	}
}
