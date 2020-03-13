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

package app

import (
	"context"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/agent/app/processors"
)

// Agent is a composition of all services / components
type Agent struct {
	processors []processors.Processor
	httpServer *http.Server
	httpAddr   atomic.Value // string, set once agent starts listening
	logger     *zap.Logger
}

// NewAgent creates the new Agent.
func NewAgent(
	processors []processors.Processor,
	httpServer *http.Server,
	logger *zap.Logger,
) *Agent {
	a := &Agent{
		processors: processors,
		httpServer: httpServer,
		logger:     logger,
	}
	a.httpAddr.Store("")
	return a
}

// GetHTTPRouter returns Gorilla HTTP router used by the agent's HTTP server.
func (a *Agent) GetHTTPRouter() *mux.Router {
	return a.httpServer.Handler.(*mux.Router)
}

// Run runs all of agent UDP and HTTP servers in separate go-routines.
// It returns an error when it's immediately apparent on startup, but
// any errors happening after starting the servers are only logged.
func (a *Agent) Run() error {
	listener, err := net.Listen("tcp", a.httpServer.Addr)
	if err != nil {
		return err
	}
	a.httpAddr.Store(listener.Addr().String())
	go func() {
		a.logger.Info("Starting jaeger-agent HTTP server", zap.Int("http-port", listener.Addr().(*net.TCPAddr).Port))
		if err := a.httpServer.Serve(listener); err != http.ErrServerClosed {
			a.logger.Error("http server failure", zap.Error(err))
		}
		a.logger.Info("agent's http server exiting")
	}()
	for _, processor := range a.processors {
		go processor.Serve()
	}
	return nil
}

// HTTPAddr returns the address that HTTP server is listening on
func (a *Agent) HTTPAddr() string {
	return a.httpAddr.Load().(string)
}

// Stop forces all agent go routines to exit.
func (a *Agent) Stop() {
	// first, close the http server, so that we don't have any more inflight requests
	a.logger.Info("shutting down agent's HTTP server", zap.String("addr", a.HTTPAddr()))
	timeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := a.httpServer.Shutdown(timeout); err != nil {
		a.logger.Error("failed to close HTTP server", zap.Error(err))
	}
	cancel()

	// then, close all processors that are called for the incoming http requests
	for _, processor := range a.processors {
		processor.Stop()
	}
}
