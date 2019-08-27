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

package servers

import (
	"sync"
	"testing"
	"time"

	athrift "github.com/apache/thrift/lib/go/thrift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics/metricstest"

	customtransport "github.com/jaegertracing/jaeger/cmd/agent/app/customtransports"
	"github.com/jaegertracing/jaeger/cmd/agent/app/servers/thriftudp"
	"github.com/jaegertracing/jaeger/cmd/agent/app/testutils"
	"github.com/jaegertracing/jaeger/thrift-gen/agent"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

func TestTBufferedServer(t *testing.T) {
	metricsFactory := metricstest.NewFactory(0)

	transport, err := thriftudp.NewTUDPServerTransport("127.0.0.1:0")
	require.NoError(t, err)

	maxPacketSize := 65000
	server, err := NewTBufferedServer(transport, maxPacketSize, metricsFactory)
	require.NoError(t, err)

	inMemReporter := testutils.NewInMemoryReporter()
	wg := sync.WaitGroup{}

	pr := func(readBuf *ReadBuf) {
		assert.NotEqual(t, 0, len(readBuf.GetBytes()))
		protoFact := athrift.NewTCompactProtocolFactory()
		trans := &customtransport.TBufferedReadTransport{}
		protocol := protoFact.GetProtocol(trans)
		protocol.Transport().Write(readBuf.GetBytes())
		handler := agent.NewAgentProcessor(inMemReporter)
		handler.Process(protocol, protocol)
		wg.Done()
	}

	server.RegisterProcessor(pr)

	go server.Serve()
	defer server.Stop()
	time.Sleep(10 * time.Millisecond) // wait for server to start serving

	hostPort := transport.Addr().String()
	client, clientCloser, err := testutils.NewZipkinThriftUDPClient(hostPort)
	require.NoError(t, err)
	defer clientCloser.Close()

	span := zipkincore.NewSpan()
	span.Name = "span1"

	wg.Add(1)
	err = client.EmitZipkinBatch([]*zipkincore.Span{span})
	require.NoError(t, err)

	waitChan := make(chan struct{})

	go func() {
		wg.Wait()
		close(waitChan)
	}()

	select {
	case <-waitChan:
		break
	case <-time.After(time.Second * 1):
		t.Fatalf("Server should have received span submission")
	}

	require.Equal(t, 1, len(inMemReporter.ZipkinSpans()))
	assert.Equal(t, "span1", inMemReporter.ZipkinSpans()[0].Name)

	// server must emit metrics
	metricsFactory.AssertCounterMetrics(t,
		metricstest.ExpectedMetric{Name: "thrift.udp.server.packets.processed", Value: 1},
	)
	metricsFactory.AssertGaugeMetrics(t,
		metricstest.ExpectedMetric{Name: "thrift.udp.server.packet_size", Value: 38},
	)
}
