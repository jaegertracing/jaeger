// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package telemetry_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/component/componenttest"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestNoopSettings(t *testing.T) {
	telset := telemetry.NoopSettings()
	assert.NotNil(t, telset.Logger)
	assert.NotNil(t, telset.Metrics)
	assert.NotNil(t, telset.MeterProvider)
	assert.NotNil(t, telset.TracerProvider)
	assert.NotNil(t, telset.Host)
	// ReportStatus is now a method, not a field - just verify it doesn't panic
	telset.ReportStatus(componentstatus.NewFatalErrorEvent(errors.New("foobar")))
}

func TestFromOtelComponent(t *testing.T) {
	otelTelset := component.TelemetrySettings{
		Logger:         zap.NewNop(),
		MeterProvider:  noopmetric.NewMeterProvider(),
		TracerProvider: nooptrace.NewTracerProvider(),
	}
	host := componenttest.NewNopHost()
	telset := telemetry.FromOtelComponent(otelTelset, host)
	assert.Equal(t, otelTelset.Logger, telset.Logger)
	assert.Equal(t, otelTelset.MeterProvider, telset.MeterProvider)
	assert.Equal(t, otelTelset.TracerProvider, telset.TracerProvider)
	assert.Equal(t, host, telset.Host)
	// ReportStatus is now a method - just verify it doesn't panic
	telset.ReportStatus(componentstatus.NewFatalErrorEvent(errors.New("foobar")))
}

func TestReportStatus_NilHost(t *testing.T) {
	telset := telemetry.Settings{
		Logger: zap.NewNop(),
	}
	// Should not panic, just log
	telset.ReportStatus(componentstatus.NewEvent(componentstatus.StatusOK))
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
