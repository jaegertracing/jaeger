// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package servers

import (
	"bytes"
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/thrift-gen/agent"
	"github.com/jaegertracing/jaeger-idl/thrift-gen/zipkincore"
	"github.com/jaegertracing/jaeger/cmd/agent/app/customtransport"
	"github.com/jaegertracing/jaeger/cmd/agent/app/testutils"
	"github.com/jaegertracing/jaeger/internal/metricstest"
)

func TestUDPServerSendReceive(t *testing.T) {
	metricsFactory := metricstest.NewFactory(0)

	const maxPacketSize = 65000
	const maxQueueSize = 100
	server, err := NewUDPServer("127.0.0.1:0", maxQueueSize, maxPacketSize, metricsFactory)
	require.NoError(t, err)
	go server.Serve()
	defer server.Stop()

	// TODO simplify this, no need to use real Thrift machinery, just a plain string will do.
	client, clientCloser, err := testutils.NewZipkinThriftUDPClient(server.Addr().String())
	require.NoError(t, err)
	defer clientCloser.Close()

	span := zipkincore.NewSpan()
	span.Name = "span1"

	for i := 0; i < 1000; i++ {
		err := client.EmitZipkinBatch(context.Background(), []*zipkincore.Span{span})
		require.NoError(t, err)

		select {
		case buf := <-server.DataChan():
			assert.Positive(t, buf.Len())
			t.Logf("received %d bytes", buf.Len())

			inMemReporter := testutils.NewInMemoryReporter()
			protoFact := thrift.NewTCompactProtocolFactoryConf(&thrift.TConfiguration{})
			trans := &customtransport.TBufferedReadTransport{}
			protocol := protoFact.GetProtocol(trans)

			_, err = buf.WriteTo(protocol.Transport())
			require.NoError(t, err)

			server.DataRecd(buf) // return to pool

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
type fakeConn struct {
	packet atomic.Int64
	wg     sync.WaitGroup
}

// Read simulates three packets received, then blocks until semaphore is released at the end of the test.
// First packet is returned as normal.
// Second packet is simulated as error.
// Third packet is returned as normal, but will be dropped as overflow by the server whose queue size = 1.
func (t *fakeConn) Read(p []byte) (n int, err error) {
	packet := t.packet.Add(1)
	if packet == 2 {
		// return some error packet, followed by valid one
		return 0, io.ErrNoProgress
	}
	if packet > 3 {
		// block after 3 packets until the server is shutdown and semaphore released
		t.wg.Wait()
		return 0, io.EOF
	}
	for i := range p {
		p[i] = byte(packet)
	}
	return len(p), nil
}

func (*fakeConn) Close() error {
	return nil
}

func TestUDPServerMetrics(t *testing.T) {
	metricsFactory := metricstest.NewFactory(0)

	const maxPacketSize = 65000
	const maxQueueSize = 1
	server, err := NewUDPServer("127.0.0.1:0", maxQueueSize, maxPacketSize, metricsFactory)
	require.NoError(t, err)

	// replace connection with fake one
	conn := new(fakeConn)
	conn.wg.Add(1)
	defer conn.wg.Done()
	server.conn.Close()
	server.conn = conn

	go server.Serve()
	defer server.Stop()

	// The fakeConn will allow the server to read exactly two packets and one error in between.
	// Since we use the server with queue size == 1, the first packet will be
	// sent to channel, the error will increment the metric, and the second valid packet dropped.

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

	var readBuf *bytes.Buffer
	select {
	case readBuf = <-server.DataChan():
		b := readBuf.Bytes()
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
