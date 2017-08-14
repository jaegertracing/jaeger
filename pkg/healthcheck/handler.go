// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package healthcheck

import (
	"net"
	"net/http"
	"strconv"

	"go.uber.org/zap"
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
	s.logger.Info("Health Check state change", zap.Int("http-port", s.state))
}

// Get the current status code for this health check
func (s *State) Get() int {
	return s.state
}
