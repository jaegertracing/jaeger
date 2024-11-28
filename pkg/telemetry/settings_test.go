// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package telemetry_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component/componentstatus"

	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/telemetry"
	"github.com/jaegertracing/jaeger/pkg/testutils"
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

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
