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
	"net"
	"net/http"
	"strconv"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/version"
)

// State represents the current health check state
type State struct {
	state   int
	logger  *zap.Logger
	onClose chan struct{}
}

// Serve requests on the specified port. The initial state is what's specified with the state parameter
func Serve(state int, port int, logger *zap.Logger) (*State, error) {
	hs, err := NewState(state, logger)
	handler, err := NewHandler(hs)

	s := &http.Server{Handler: handler}
	portStr := ":" + strconv.Itoa(port)
	l, err := net.Listen("tcp", portStr)
	if err != nil {
		logger.Error("failed to listen", zap.Error(err))
		return nil, err
	}
	ServeWithListener(l, s, logger)

	hs.logger.Info("Health Check server started", zap.Int("http-port", port))

	return hs, err
}

// NewState creates a new state instance. The initial state is what's specified with the state parameter
func NewState(state int, logger *zap.Logger) (*State, error) {
	s := &State{state: state, logger: logger}
	return s, nil
}

// NewHandler creates a new HTTP handler using the given state as source of information
func NewHandler(s *State) (http.Handler, error) {
	mu := http.NewServeMux()
	mu.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(s.state)
		// this is written only for response with an entity, so, it won't be used for a 204 - No content
		w.Write([]byte("Server not available"))
	})

	version.RegisterHandler(mu, s.logger)
	return mu, nil
}

// ServeWithListener starts a new HTTP server on the given port and with the provided handler
func ServeWithListener(l net.Listener, s *http.Server, logger *zap.Logger) (*http.Server, error) {
	go func() {
		if err := s.Serve(l); err != nil {
			logger.Error("failed to serve", zap.Error(err))
		}
	}()

	return s, nil
}

// Ready updates the current state to the "ready" state, translated as a 2xx-class HTTP status code
func (s *State) Ready() {
	s.Set(http.StatusNoContent)
}

// Set a new HTTP status for the health check
func (s *State) Set(state int) {
	s.state = state
	s.logger.Info("Health Check state change", zap.Int("http-status", s.state))
}

// Get the current status code for this health check
func (s *State) Get() int {
	return s.state
}
