// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package healthcheck

import (
	"net/http"
	"sync/atomic"

	"go.uber.org/zap"
)

// Status represents the state of the service.
type Status int

const (
	// Unavailable indicates the service is not able to handle requests
	Unavailable Status = iota
	// Ready indicates the service is ready to handle requests
	Ready
	// Broken indicates that the healthcheck itself is broken, not serving HTTP
	Broken
)

func (s Status) String() string {
	switch s {
	case Unavailable:
		return "unavailable"
	case Ready:
		return "ready"
	case Broken:
		return "broken"
	default:
		return "unknown"
	}
}

// HealthCheck provides an HTTP endpoint that returns the health status of the service
type HealthCheck struct {
	state   int32 // atomic, keep at the top to be word-aligned
	logger  *zap.Logger
	mapping map[Status]int
	server  *http.Server
}

// New creates a HealthCheck with the specified initial state.
func New() *HealthCheck {
	hc := &HealthCheck{
		state: int32(Unavailable),
		mapping: map[Status]int{
			Unavailable: http.StatusServiceUnavailable,
			Ready:       http.StatusNoContent,
		},
		logger: zap.NewNop(),
	}
	return hc
}

// SetLogger initializes a logger.
func (hc *HealthCheck) SetLogger(logger *zap.Logger) {
	hc.logger = logger
}

// Handler creates a new HTTP handler.
func (hc *HealthCheck) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(hc.mapping[hc.Get()])
		// this is written only for response with an entity, so, it won't be used for a 204 - No content
		w.Write([]byte("Server not available"))
	})
}

// Set a new health check status
func (hc *HealthCheck) Set(state Status) {
	atomic.StoreInt32(&hc.state, int32(state))
	hc.logger.Info("Health Check state change", zap.Stringer("status", hc.Get()))
}

// Get the current status of this health check
func (hc *HealthCheck) Get() Status {
	return Status(atomic.LoadInt32(&hc.state))
}

// Ready is a shortcut for Set(Ready) (kept for backwards compatibility)
func (hc *HealthCheck) Ready() {
	hc.Set(Ready)
}
