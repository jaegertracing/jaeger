package agent

import (
	"github.com/uber-go/zap"
)

// Agent is a composition of all services / components
type Agent interface {
	// Run starts all services of the Agent
	// Run()
}

type agent struct {
	logger zap.Logger
}

// New creates a new Jaeger Agent
func New(
	logger zap.Logger,
) Agent {
	return &agent{
		logger: logger,
	}
}
