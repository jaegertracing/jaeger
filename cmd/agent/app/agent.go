package app

import (
	"io"
	"net"
	"net/http"

	"github.com/uber-go/zap"

	"code.uber.internal/infra/jaeger/oss/cmd/agent/app/processors"
)

// Agent is a composition of all services / components
type Agent struct {
	processors      []processors.Processor
	samplingServer  *http.Server
	discoveryClient interface{}
	logger          zap.Logger
	closer          io.Closer
}

// NewAgent creates the new Agent.
func NewAgent(
	processors []processors.Processor,
	samplingServer *http.Server,
	discoveryClient interface{},
	logger zap.Logger,
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
