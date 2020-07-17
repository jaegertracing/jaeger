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
	"context"
	"io"
	"sync"
	"testing"
	"time"

	athrift "github.com/apache/thrift/lib/go/thrift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics/metricstest"
	"go.uber.org/atomic"

	"github.com/jaegertracing/jaeger/cmd/agent/app/customtransport"
	"github.com/jaegertracing/jaeger/cmd/agent/app/servers/thriftudp"
	"github.com/jaegertracing/jaeger/cmd/agent/app/testutils"
	"github.com/jaegertracing/jaeger/thrift-gen/agent"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

func TestTBufferedServer_SendReceive(t *testing.T) {
	metricsFactory := metricstest.NewFactory(0)

	transport, err := thriftudp.NewTUDPServerTransport("127.0.0.1:0")
	require.NoError(t, err)

	maxPacketSize := 65000
	server, err := NewTBufferedServer(transport, 100, maxPacketSize, metricsFactory)
	require.NoError(t, err)
	go server.Serve()
	defer server.Stop()

	hostPort := transport.Addr().String()
	client, clientCloser, err := testutils.NewZipkinThriftUDPClient(hostPort)
	require.NoError(t, err)
	defer clientCloser.Close()

	span := zipkincore.NewSpan()
	span.Name = "span1"

	for i := 0; i < 1000; i++ {
		err := client.EmitZipkinBatch(context.Background(), []*zipkincore.Span{span})
		require.NoError(t, err)

		select {
		case readBuf := <-server.DataChan():
			assert.NotEqual(t, 0, len(readBuf.GetBytes()))

			inMemReporter := testutils.NewInMemoryReporter()
			protoFact := athrift.NewTCompactProtocolFactory()
			trans := &customtransport.TBufferedReadTransport{}
			protocol := protoFact.GetProtocol(trans)

			_, err = protocol.Transport().Write(readBuf.GetBytes())
			require.NoError(t, err)

			server.DataRecd(readBuf) // return to pool

			handler := agent.NewAgentProcessor(inMemReporter)
			_, err = handler.Process(context.Background(), protocol, protocol)
			require.NoError(t, err)

			require.Len(t, inMemReporter.ZipkinSpans(), 1)
			assert.Equal(t, "span1", inMemReporter.ZipkinSpans()[0].Name)

			return // exit test on successful receipt
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	t.Fatal("server did not receive packets")
}

// The fakeTransport allows the server to read two packets, one filled with 1's, another with 2's,
// then returns an error, and then blocks on the semaphore. The semaphore is only released when
// the test is exiting.
type fakeTransport struct {
	packet atomic.Int64
	wg     sync.WaitGroup
}

func (t *fakeTransport) Read(p []byte) (n int, err error) {
	packet := t.packet.Inc()
	if packet > 2 {
		if packet > 3 {
			// return error once when packet==3, otherwise block
			t.wg.Wait()
		}
		return 0, io.EOF
	}
	for i := range p {
		p[i] = byte(packet)
	}
	return len(p), nil
}

func (t *fakeTransport) Close() error {
	return nil
}

func TestTBufferedServer_Metrics(t *testing.T) {
	metricsFactory := metricstest.NewFactory(0)

	transport := new(fakeTransport)
	transport.wg.Add(1)
	defer transport.wg.Done()

	maxPacketSize := 65000
	server, err := NewTBufferedServer(transport, 1, maxPacketSize, metricsFactory)
	require.NoError(t, err)
	go server.Serve()
	defer server.Stop()

	// The fakeTransport will allow the server to read exactly two packets and one error.
	// Since we use the server with queue size == 1, the first packet will be
	// sent to channel, and the second one dropped.

	packetDropped := false
	for i := 0; i < 5000; i++ {
		c, _ := metricsFactory.Snapshot()
		if c["thrift.udp.server.packets.dropped"] == 1 {
			packetDropped = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	require.True(t, packetDropped, "packetDropped")

	var readBuf *ReadBuf
	select {
	case readBuf = <-server.DataChan():
		b := readBuf.GetBytes()
		assert.Len(t, b, 65000)
		assert.EqualValues(t, 1, b[0], "first packet must be all 0x01's")
	default:
		t.Fatal("expecting a packet in the channel")
	}

	metricsFactory.AssertCounterMetrics(t,
		metricstest.ExpectedMetric{Name: "thrift.udp.server.packets.processed", Value: 1},
		metricstest.ExpectedMetric{Name: "thrift.udp.server.packets.dropped", Value: 1},
		metricstest.ExpectedMetric{Name: "thrift.udp.server.read.errors", Value: 1},
	)
	metricsFactory.AssertGaugeMetrics(t,
		metricstest.ExpectedMetric{Name: "thrift.udp.server.packet_size", Value: 65000},
		metricstest.ExpectedMetric{Name: "thrift.udp.server.queue_size", Value: 1},
	)

	server.DataRecd(readBuf)
	metricsFactory.AssertGaugeMetrics(t,
		metricstest.ExpectedMetric{Name: "thrift.udp.server.queue_size", Value: 0},
	)
}
