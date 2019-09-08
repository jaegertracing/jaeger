// Copyright (c) 2019 The Jaeger Authors.
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
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

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

type healthCheckResponse struct {
	statusCode int
	StatusMsg  string    `json:"status"`
	UpSince    time.Time `json:"upSince"`
	Uptime     string    `json:"uptime"`
}

type state struct {
	status  Status
	upSince time.Time
}

// HealthCheck provides an HTTP endpoint that returns the health status of the service
type HealthCheck struct {
	state     atomic.Value // stores state struct
	logger    *zap.Logger
	responses map[Status]healthCheckResponse
}

// New creates a HealthCheck with the specified initial state.
func New() *HealthCheck {
	hc := &HealthCheck{
		logger: zap.NewNop(),
		responses: map[Status]healthCheckResponse{
			Unavailable: {
				statusCode: http.StatusServiceUnavailable,
				StatusMsg:  "Server not available",
			},
			Ready: {
				statusCode: http.StatusOK,
				StatusMsg:  "Server available",
			},
		},
	}
	hc.state.Store(state{status: Unavailable})
	return hc
}

// SetLogger initializes a logger.
func (hc *HealthCheck) SetLogger(logger *zap.Logger) {
	hc.logger = logger
}

// Handler creates a new HTTP handler.
func (hc *HealthCheck) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		state := hc.getState()
		template := hc.responses[state.status]
		w.WriteHeader(template.statusCode)

		w.Header().Set("Content-Type", "application/json")
		w.Write(hc.createRespBody(state, template))
	})
}

func (hc *HealthCheck) createRespBody(state state, template healthCheckResponse) []byte {
	resp := template // clone
	if state.status == Ready {
		resp.UpSince = state.upSince
		resp.Uptime = fmt.Sprintf("%v", time.Since(state.upSince))
	}
	healthCheckStatus, _ := json.Marshal(resp)
	return healthCheckStatus
}

// Set a new health check status
func (hc *HealthCheck) Set(status Status) {
	oldState := hc.getState()
	newState := state{status: status}
	if status == Ready {
		if oldState.status != Ready {
			newState.upSince = time.Now()
		}
	}
	hc.state.Store(newState)
	hc.logger.Info("Health Check state change", zap.Stringer("status", status))
}

// Get the current status of this health check
func (hc *HealthCheck) Get() Status {
	return hc.getState().status
}

func (hc *HealthCheck) getState() state {
	return hc.state.Load().(state)
}

// Ready is a shortcut for Set(Ready) (kept for backwards compatibility)
func (hc *HealthCheck) Ready() {
	hc.Set(Ready)
}
