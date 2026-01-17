// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package flags

import (
	"net/http"
	"sync/atomic"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
)

var (
	_ component.Host           = (*HealthHost)(nil)
	_ componentstatus.Reporter = (*HealthHost)(nil)
)

// HealthHost implements component.Host and componentstatus.Reporter
// and provides an HTTP handler for health checks.
type HealthHost struct {
	ready atomic.Bool
}

// NewHealthHost creates a new HealthHost in not-ready state.
func NewHealthHost() *HealthHost {
	return &HealthHost{}
}

// GetExtensions implements component.Host.
func (*HealthHost) GetExtensions() map[component.ID]component.Component {
	return nil
}

// Report implements componentstatus.Reporter.
func (h *HealthHost) Report(event *componentstatus.Event) {
	switch event.Status() {
	case componentstatus.StatusOK, componentstatus.StatusRecoverableError:
		h.Ready()
	default:
		h.SetUnavailable()
	}
}

// Ready sets the health status to ready.
func (h *HealthHost) Ready() {
	h.ready.Store(true)
}

// SetUnavailable sets the health status to unavailable.
func (h *HealthHost) SetUnavailable() {
	h.ready.Store(false)
}

// Handler returns an HTTP handler for the health endpoint.
// Returns 204 No Content when ready, 503 Service Unavailable otherwise.
func (h *HealthHost) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if h.ready.Load() {
			w.WriteHeader(http.StatusNoContent)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	})
}
