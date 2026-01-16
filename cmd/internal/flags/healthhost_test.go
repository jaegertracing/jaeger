// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package flags

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component/componentstatus"
)

func TestHealthHost(t *testing.T) {
	hh := NewHealthHost()

	// Initially unavailable
	assert.Equal(t, Unavailable, hh.Get())

	// Set to Ready
	hh.Ready()
	assert.Equal(t, Ready, hh.Get())

	// Set to Unavailable
	hh.SetUnavailable()
	assert.Equal(t, Unavailable, hh.Get())

	// Set directly
	hh.Set(Ready)
	assert.Equal(t, Ready, hh.Get())
}

func TestHealthHost_Report(t *testing.T) {
	hh := NewHealthHost()

	// StatusOK should set Ready
	hh.Report(componentstatus.NewEvent(componentstatus.StatusOK))
	assert.Equal(t, Ready, hh.Get())

	// StatusStopping should set Unavailable
	hh.Report(componentstatus.NewEvent(componentstatus.StatusStopping))
	assert.Equal(t, Unavailable, hh.Get())

	// StatusRecoverableError should set Ready
	hh.Report(componentstatus.NewRecoverableErrorEvent(nil))
	assert.Equal(t, Ready, hh.Get())

	// StatusFatalError should set Unavailable
	hh.Report(componentstatus.NewFatalErrorEvent(nil))
	assert.Equal(t, Unavailable, hh.Get())
}

func TestHealthHost_Handler(t *testing.T) {
	hh := NewHealthHost()
	handler := hh.Handler()

	// When unavailable, should return 503
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	// When ready, should return 204
	hh.Ready()
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestHealthHost_GetExtensions(t *testing.T) {
	hh := NewHealthHost()
	// Should return nil (no extensions)
	assert.Nil(t, hh.GetExtensions())
}

func TestHealthStatus_String(t *testing.T) {
	assert.Equal(t, "unavailable", Unavailable.String())
	assert.Equal(t, "ready", Ready.String())
	assert.Equal(t, "unknown", HealthStatus(99).String())
}
