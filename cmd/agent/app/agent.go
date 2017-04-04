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

package app

import (
	"io"
	"net"
	"net/http"

	"go.uber.org/zap"

	"github.com/uber/jaeger/cmd/agent/app/processors"
)

// Agent is a composition of all services / components
type Agent struct {
	processors      []processors.Processor
	samplingServer  *http.Server
	discoveryClient interface{}
	logger          *zap.Logger
	closer          io.Closer
}

// NewAgent creates the new Agent.
func NewAgent(
	processors []processors.Processor,
	samplingServer *http.Server,
	discoveryClient interface{},
	logger *zap.Logger,
) *Agent {
	return &Agent{
		processors:      processors,
		samplingServer:  samplingServer,
		discoveryClient: discoveryClient,
		logger:          logger,
	}
}

// Run runs all of agent UDP and HTTP servers in separate go-routines.
// It returns an error when it's immediately apparent on startup, but
// any errors happening after starting the servers are only logged.
func (a *Agent) Run() error {
	listener, err := net.Listen("tcp", a.samplingServer.Addr)
	if err != nil {
		return err
	}
	a.closer = listener
	go func() {
		if err := a.samplingServer.Serve(listener); err != nil {
			a.logger.Error("sampling server failure", zap.Error(err))
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
