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

	"github.com/jaegertracing/jaeger/internal/healthcheck"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestHCAdapter(t *testing.T) {
	tests := []struct {
		name       string
		status     componentstatus.Status
		expectedHC healthcheck.Status
	}{
		{
			name:       "StatusOK",
			status:     componentstatus.StatusOK,
			expectedHC: healthcheck.Ready,
		},
		{
			name:       "StatusStarting",
			status:     componentstatus.StatusStarting,
			expectedHC: healthcheck.Unavailable,
		},
		{
			name:       "StatusRecoverableError",
			status:     componentstatus.StatusRecoverableError,
			expectedHC: healthcheck.Unavailable,
		},
		{
			name:       "StatusPermanentError",
			status:     componentstatus.StatusPermanentError,
			expectedHC: healthcheck.Unavailable,
		},
		{
			name:       "StatusNone",
			status:     componentstatus.StatusNone,
			expectedHC: healthcheck.Unavailable,
		},
		{
			name:       "StatusStopping",
			status:     componentstatus.StatusStopping,
			expectedHC: healthcheck.Unavailable,
		},
		{
			name:       "StatusStopped",
			status:     componentstatus.StatusStopped,
			expectedHC: healthcheck.Unavailable,
		},
		{
			name:       "StatusFatalError",
			status:     componentstatus.StatusFatalError,
			expectedHC: healthcheck.Broken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hc := healthcheck.New()
			hcAdapter := telemetry.HCAdapter(hc)
			event := componentstatus.NewEvent(tt.status)
			hcAdapter(event)
			assert.Equal(t, tt.expectedHC, hc.Get())
		})
	}
}

func TestNoopSettingss(t *testing.T) {
	telset := telemetry.NoopSettings()
	assert.NotNil(t, telset.Logger)
	assert.NotNil(t, telset.Metrics)
	assert.NotNil(t, telset.MeterProvider)
	assert.NotNil(t, telset.TracerProvider)
	assert.NotNil(t, telset.ReportStatus)
	assert.NotNil(t, telset.Host)
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
	assert.NotNil(t, telset.ReportStatus)
	telset.ReportStatus(componentstatus.NewFatalErrorEvent(errors.New("foobar")))
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
