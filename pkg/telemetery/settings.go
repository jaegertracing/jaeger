// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package telemetery

import (
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/jtracer"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

type Setting struct {
	Logger       *zap.Logger
	Tracer       *jtracer.JTracer
	Metrics      metrics.Factory
	ReportStatus func(*StatusEvent)
}

type StatusEvent struct {
	status healthcheck.Status
}

func HCAdapter(hc *healthcheck.HealthCheck) func(*StatusEvent) {
	return func(event *StatusEvent) {
		hc.Set(event.status)
	}
}
