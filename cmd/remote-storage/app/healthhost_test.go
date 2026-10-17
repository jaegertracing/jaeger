// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componentstatus"

	"github.com/jaegertracing/jaeger/cmd/internal/status"
)

func TestHealthHost_Handler(t *testing.T) {
	hh := NewHealthHost()
	handler := hh.Handler()

	// Initially unavailable - should return 503
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	// Set to ready - should return 204
	hh.Ready()
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Set to unavailable - should return 503
	hh.SetUnavailable()
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHealthHost_Report(t *testing.T) {
	hh := NewHealthHost()
	handler := hh.Handler()
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)

	// StatusOK should set Ready
	hh.Report(componentstatus.NewEvent(componentstatus.StatusOK))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// StatusStopping should set Unavailable
	hh.Report(componentstatus.NewEvent(componentstatus.StatusStopping))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	// StatusRecoverableError should set Ready
	hh.Report(componentstatus.NewRecoverableErrorEvent(nil))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// StatusFatalError should set Unavailable
	hh.Report(componentstatus.NewFatalErrorEvent(nil))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHealthHost_GetExtensions(t *testing.T) {
	hh := NewHealthHost()
	assert.Nil(t, hh.GetExtensions())
}

// TestHealthHost_StatusCommand runs the `status` command against the real health
// handler, because the two are wired into the same binary and must agree on what
// a healthy response looks like.
func TestHealthHost_StatusCommand(t *testing.T) {
	hh := NewHealthHost()
	ts := httptest.NewServer(hh.Handler())
	defer ts.Close()

	runStatus := func() (string, error) {
		cmd := status.Command(viper.New(), 80)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"--status.http.host-port=" + strings.TrimPrefix(ts.URL, "http://")})
		err := cmd.Execute()
		return out.String(), err
	}

	_, err := runStatus()
	require.ErrorContains(t, err, "503")

	hh.Ready()
	out, err := runStatus()
	require.NoError(t, err)
	assert.Equal(t, "204 No Content\n", out)
}
