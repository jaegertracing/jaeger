// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package testutils

import (
	"io"

	"github.com/apache/thrift/lib/go/thrift"

	"github.com/jaegertracing/jaeger/cmd/agent/app/servers/thriftudp"
	"github.com/jaegertracing/jaeger-idl/thrift-gen/agent"
)

// NewZipkinThriftUDPClient creates a new zipking agent client that works like Jaeger client
func NewZipkinThriftUDPClient(hostPort string) (*agent.AgentClient, io.Closer, error) {
	clientTransport, err := thriftudp.NewTUDPClientTransport(hostPort, "")
	if err != nil {
		return nil, nil, err
	}

	protocolFactory := thrift.NewTCompactProtocolFactoryConf(&thrift.TConfiguration{})
	client := agent.NewAgentClientFactory(clientTransport, protocolFactory)
	return client, clientTransport, nil
}

// NewJaegerThriftUDPClient creates a new jaeger agent client that works like Jaeger client
func NewJaegerThriftUDPClient(hostPort string, protocolFactory thrift.TProtocolFactory) (*agent.AgentClient, io.Closer, error) {
	clientTransport, err := thriftudp.NewTUDPClientTransport(hostPort, "")
	if err != nil {
		return nil, nil, err
	}

	client := agent.NewAgentClientFactory(clientTransport, protocolFactory)
	return client, clientTransport, nil
}
