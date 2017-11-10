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

package testutils

import (
	"io"

	"github.com/apache/thrift/lib/go/thrift"

	"github.com/jaegertracing/jaeger/cmd/agent/app/servers/thriftudp"
	"github.com/jaegertracing/jaeger/thrift-gen/agent"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
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
