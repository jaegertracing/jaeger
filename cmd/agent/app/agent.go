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
	"io"
	"net"
	"net/http"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/agent/app/processors"
)

// Agent is a composition of all services / components
type Agent struct {
	processors []processors.Processor
	httpServer *http.Server
	logger     *zap.Logger
	closer     io.Closer
}

// NewAgent creates the new Agent.
func NewAgent(
	processors []processors.Processor,
	httpServer *http.Server,
	logger *zap.Logger,
) *Agent {
	return &Agent{
		processors: processors,
		httpServer: httpServer,
		logger:     logger,
	}
}

// Run runs all of agent UDP and HTTP servers in separate go-routines.
// It returns an error when it's immediately apparent on startup, but
// any errors happening after starting the servers are only logged.
func (a *Agent) Run() error {
	listener, err := net.Listen("tcp", a.httpServer.Addr)
	if err != nil {
		return err
	}
	a.closer = listener
	go func() {
		if err := a.httpServer.Serve(listener); err != nil {
			a.logger.Error("http server failure", zap.Error(err))
		}
	}()
	for _, processor := range a.processors {
		go processor.Serve()
	}
	return nil
}

// Stop forces all agent go routines to exit.
func (a *Agent) Stop() {
	for _, processor := range a.processors {
		go processor.Stop()
	}
	a.closer.Close()
}
