// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package telemetery_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component"

	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/telemetery"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestHCAdapter(t *testing.T) {
	tests := []struct {
		name       string
		status     component.Status
		expectedHC healthcheck.Status
	}{
		{
			name:       "StatusOK",
			status:     component.StatusOK,
			expectedHC: healthcheck.Ready,
		},
		{
			name:       "StatusStarting",
			status:     component.StatusStarting,
			expectedHC: healthcheck.Unavailable,
		},
		{
			name:       "StatusRecoverableError",
			status:     component.StatusRecoverableError,
			expectedHC: healthcheck.Unavailable,
		},
		{
			name:       "StatusPermanentError",
			status:     component.StatusPermanentError,
			expectedHC: healthcheck.Unavailable,
		},
		{
			name:       "StatusNone",
			status:     component.StatusNone,
			expectedHC: healthcheck.Unavailable,
		},
		{
			name:       "StatusStopping",
			status:     component.StatusStopping,
			expectedHC: healthcheck.Unavailable,
		},
		{
			name:       "StatusStopped",
			status:     component.StatusStopped,
			expectedHC: healthcheck.Unavailable,
		},
		{
			name:       "StatusFatalError",
			status:     component.StatusFatalError,
			expectedHC: healthcheck.Broken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hc := healthcheck.New()
			hcAdapter := telemetery.HCAdapter(hc)
			event := component.NewStatusEvent(tt.status)
			hcAdapter(event)
			assert.Equal(t, tt.expectedHC, hc.Get())
		})
	}
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
