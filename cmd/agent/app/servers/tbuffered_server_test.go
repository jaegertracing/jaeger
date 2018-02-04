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
	"testing"
	"time"

	athrift "github.com/apache/thrift/lib/go/thrift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	mTestutils "github.com/uber/jaeger-lib/metrics/testutils"

	"github.com/jaegertracing/jaeger/cmd/agent/app/customtransports"
	"github.com/jaegertracing/jaeger/cmd/agent/app/servers/thriftudp"
	"github.com/jaegertracing/jaeger/cmd/agent/app/testutils"
	"github.com/jaegertracing/jaeger/thrift-gen/agent"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

func TestTBufferedServer(t *testing.T) {
	t.Run("processed", func(t *testing.T) {
		testTBufferedServer(t, 10, false)
	})
	t.Run("dropped", func(t *testing.T) {
		testTBufferedServer(t, 1, true)
	})
}

func testTBufferedServer(t *testing.T, queueSize int, testDroppedPackets bool) {
	metricsFactory := metrics.NewLocalFactory(0)

	transport, err := thriftudp.NewTUDPServerTransport("127.0.0.1:0")
	require.NoError(t, err)

	maxPacketSize := 65000
	server, err := NewTBufferedServer(transport, queueSize, maxPacketSize, metricsFactory)
	require.NoError(t, err)
	go server.Serve()
	defer server.Stop()
	time.Sleep(10 * time.Millisecond) // wait for server to start serving

	hostPort := transport.Addr().String()
	client, clientCloser, err := testutils.NewZipkinThriftUDPClient(hostPort)
	require.NoError(t, err)
	defer clientCloser.Close()

	span := zipkincore.NewSpan()
	span.Name = "span1"

	err = client.EmitZipkinBatch([]*zipkincore.Span{span})
	require.NoError(t, err)

	if testDroppedPackets {
		// because queueSize == 1 for this test, and we're not reading from data chan,
		// the second packet we send will be dropped by the server
		err = client.EmitZipkinBatch([]*zipkincore.Span{span})
		require.NoError(t, err)

		for i := 0; i < 50; i++ {
			c, _ := metricsFactory.Snapshot()
			if c["thrift.udp.server.packets.dropped"] == 1 {
				return
			}
			time.Sleep(time.Millisecond)
		}
		c, _ := metricsFactory.Snapshot()
		assert.FailNow(t, "Dropped packets counter not incremented", "Counters: %+v", c)
	}

	inMemReporter := testutils.NewInMemoryReporter()
	select {
	case readBuf := <-server.DataChan():
		assert.NotEqual(t, 0, len(readBuf.GetBytes()))
		protoFact := athrift.NewTCompactProtocolFactory()
		trans := &customtransport.TBufferedReadTransport{}
		protocol := protoFact.GetProtocol(trans)
		protocol.Transport().Write(readBuf.GetBytes())
		server.DataRecd(readBuf)
		handler := agent.NewAgentProcessor(inMemReporter)
		handler.Process(protocol, protocol)
	case <-time.After(time.Second * 1):
		t.Fatalf("Server should have received span submission")
	}

	require.Equal(t, 1, len(inMemReporter.ZipkinSpans()))
	assert.Equal(t, "span1", inMemReporter.ZipkinSpans()[0].Name)

	// server must emit metrics
	mTestutils.AssertCounterMetrics(t, metricsFactory,
		mTestutils.ExpectedMetric{Name: "thrift.udp.server.packets.processed", Value: 1},
		mTestutils.ExpectedMetric{Name: "thrift.udp.server.packets.dropped", Value: 0},
	)
	mTestutils.AssertGaugeMetrics(t, metricsFactory,
		mTestutils.ExpectedMetric{Name: "thrift.udp.server.packet_size", Value: 38},
		mTestutils.ExpectedMetric{Name: "thrift.udp.server.queue_size", Value: 0},
	)
}
