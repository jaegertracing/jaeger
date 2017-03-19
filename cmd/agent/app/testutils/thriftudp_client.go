package testutils

import (
	"io"

	"github.com/apache/thrift/lib/go/thrift"

	"code.uber.internal/infra/jaeger/oss/cmd/agent/app/servers/thriftudp"
	"github.com/uber/jaeger/thrift-gen/agent"
	"github.com/uber/jaeger/thrift-gen/jaeger"
)

// NewZipkinThriftUDPClient creates a new zipking agent client that works like Jaeger client
func NewZipkinThriftUDPClient(hostPort string) (*agent.AgentClient, io.Closer, error) {
	clientTransport, err := thriftudp.NewTUDPClientTransport(hostPort, "")
	if err != nil {
		return nil, nil, err
	}

	protocolFactory := thrift.NewTCompactProtocolFactory()
	client := agent.NewAgentClientFactory(clientTransport, protocolFactory)
	return client, clientTransport, nil
}

// NewJaegerThriftUDPClient creates a new jaeger agent client that works like Jaeger client
func NewJaegerThriftUDPClient(hostPort string, protocolFactory thrift.TProtocolFactory) (*jaeger.AgentClient, io.Closer, error) {
	clientTransport, err := thriftudp.NewTUDPClientTransport(hostPort, "")
	if err != nil {
		return nil, nil, err
	}

	client := jaeger.NewAgentClientFactory(clientTransport, protocolFactory)
	return client, clientTransport, nil
}
