package agent_test

import (
	"testing"

	"github.com/uber/jaeger/services/agent"
)

func TestNewAgent(t *testing.T) {
	agent.New(nil)
}
