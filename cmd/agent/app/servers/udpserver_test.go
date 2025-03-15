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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/agent/app/servers/thriftudp"
	"github.com/jaegertracing/jaeger/internal/metricstest"
)

func TestTBufferedServerSendReceive(t *testing.T) {
	metricsFactory := metricstest.NewFactory(0)

	transport, err := thriftudp.NewTUDPServerTransport("127.0.0.1:0")
	require.NoError(t, err)

	const maxPacketSize = 65000
	const maxQueueSize = 100
	server, err := NewUDPServer(transport, maxQueueSize, maxPacketSize, metricsFactory)
	require.NoError(t, err)
	go server.Serve()
	defer server.Stop()

	client, err := thriftudp.NewTUDPClientTransport(transport.Addr().String(), "")
	require.NoError(t, err)
	defer client.Close()

	// keep sending packets until the server receives one
	for range 1000 {
		n, err := client.Write([]byte("span1"))
		require.NoError(t, err)
		require.Equal(t, 5, n)
		require.NoError(t, client.Flush(context.Background()))

		select {
		case buf := <-server.DataChan():
			assert.Positive(t, buf.Len())
			assert.Equal(t, "span1", buf.String())
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

// Read simulates three packets received, then blocks until semaphore is released at the end of the test.
// First packet is returned as normal.
// Second packet is simulated as error.
// Third packet is returned as normal, but will be dropped as overflow by the server whose queue size = 1.
func (t *fakeTransport) Read(p []byte) (n int, err error) {
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

func (*fakeTransport) Close() error {
	return nil
}

func TestTBufferedServerMetrics(t *testing.T) {
	metricsFactory := metricstest.NewFactory(0)

	transport := new(fakeTransport)
	transport.wg.Add(1)
	defer transport.wg.Done()

	const maxPacketSize = 65000
	const maxQueueSize = 1
	server, err := NewUDPServer(transport, maxQueueSize, maxPacketSize, metricsFactory)
	require.NoError(t, err)
	go server.Serve()
	defer server.Stop()

	// The fakeTransport will allow the server to read exactly two packets and one error in between.
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
