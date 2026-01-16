// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package flags

import (
	"net/http"
	"sync/atomic"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
)

// HealthStatus represents the health state of the service.
type HealthStatus int32

const (
	Unavailable HealthStatus = iota
	Ready
)

func (s HealthStatus) String() string {
	switch s {
	case Ready:
		return "ready"
	case Unavailable:
		return "unavailable"
	default:
		return "unknown"
	}
}

var (
	_ component.Host           = (*HealthHost)(nil)
	_ componentstatus.Reporter = (*HealthHost)(nil)
)

// HealthHost implements component.Host and componentstatus.Reporter
// and provides an HTTP handler for health checks.
type HealthHost struct {
	status atomic.Int32
}

// NewHealthHost creates a new HealthHost in Unavailable state.
func NewHealthHost() *HealthHost {
	return &HealthHost{}
}

// GetExtensions implements component.Host.
func (*HealthHost) GetExtensions() map[component.ID]component.Component {
	return nil
}

// Report implements componentstatus.Reporter.
// It maps OTEL component status to simple Ready/Unavailable health status.
func (h *HealthHost) Report(event *componentstatus.Event) {
	switch event.Status() {
	case componentstatus.StatusOK, componentstatus.StatusRecoverableError:
		h.Set(Ready)
	default:
		h.Set(Unavailable)
	}
}

// Get returns the current health status.
func (h *HealthHost) Get() HealthStatus {
	return HealthStatus(h.status.Load())
}

// Set sets the health status.
func (h *HealthHost) Set(status HealthStatus) {
	h.status.Store(int32(status))
}

// Ready sets the status to Ready.
func (h *HealthHost) Ready() {
	h.Set(Ready)
}

// SetUnavailable sets the status to Unavailable.
func (h *HealthHost) SetUnavailable() {
	h.Set(Unavailable)
}

// Handler returns an HTTP handler for the health endpoint.
// Returns 204 No Content when ready, 503 Service Unavailable otherwise.
func (h *HealthHost) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if h.Get() == Ready {
			w.WriteHeader(http.StatusNoContent)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	})
}
