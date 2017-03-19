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

package testutils

import (
	"io"

	"github.com/apache/thrift/lib/go/thrift"

	"github.com/uber/jaeger/cmd/agent/app/servers/thriftudp"
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
