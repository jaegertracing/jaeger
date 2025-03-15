// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package servers

import (
	"bytes"
	"io"
	"sync"
	"sync/atomic"

	"github.com/jaegertracing/jaeger/pkg/metrics"
)

// ThriftTransport is a subset of thrift.TTransport methods, for easier mocking.
type ThriftTransport interface {
	io.Reader
	io.Closer
}

// TBufferedServer is a custom thrift server that reads traffic using the transport provided
// and places messages into a buffered channel to be processed by the processor provided
type TBufferedServer struct {
	// NB. queueLength HAS to be at the top of the struct or it will SIGSEV for certain architectures.
	// See https://github.com/golang/go/issues/13868
	queueSize     int64
	dataChan      chan *bytes.Buffer
	maxPacketSize int
	maxQueueSize  int
	serving       uint32
	transport     ThriftTransport
	readBufPool   sync.Pool
	metrics       struct {
		// Size of the current server queue
		QueueSize metrics.Gauge `metric:"thrift.udp.server.queue_size"`

		// Size (in bytes) of packets received by server
		PacketSize metrics.Gauge `metric:"thrift.udp.server.packet_size"`

		// Number of packets dropped by server
		PacketsDropped metrics.Counter `metric:"thrift.udp.server.packets.dropped"`

		// Number of packets processed by server
		PacketsProcessed metrics.Counter `metric:"thrift.udp.server.packets.processed"`

		// Number of malformed packets the server received
		ReadError metrics.Counter `metric:"thrift.udp.server.read.errors"`
	}
}

// state values for TBufferedServer.serving
//
// init -> serving -> stopped
// init -> stopped (might happen in unit tests)
const (
	stateStopped = iota
	stateServing
	stateInit
)

// NewTBufferedServer creates a TBufferedServer
func NewTBufferedServer(
	transport ThriftTransport,
	maxQueueSize int,
	maxPacketSize int,
	mFactory metrics.Factory,
) (*TBufferedServer, error) {
	dataChan := make(chan *bytes.Buffer, maxQueueSize)

	res := &TBufferedServer{
		dataChan:      dataChan,
		transport:     transport,
		maxQueueSize:  maxQueueSize,
		maxPacketSize: maxPacketSize,
		serving:       stateInit,
		readBufPool: sync.Pool{
			New: func() any {
				return new(bytes.Buffer)
			},
		},
	}

	metrics.MustInit(&res.metrics, mFactory, nil)
	return res, nil
}

// packetReader is a helper for reading a single packet no larger than maxPacketSize
// from the underlying reader. Without it the ReadFrom() method of bytes.Buffer would
// read multiple packets and won't even stop  at maxPacketSize.
type packetReader struct {
	maxPacketSize int
	reader        *io.LimitedReader
	attempt       int
}

func (r *packetReader) Read(p []byte) (int, error) {
	if r.attempt > 0 {
		return 0, io.EOF
	}
	r.attempt = 1
	return r.reader.Read(p)
}

func (r *packetReader) readPacket(buf *bytes.Buffer) (int, error) {
	// reset the readers since we're reusing them to avoid allocations
	r.attempt = 0
	r.reader.N = int64(r.maxPacketSize)
	// prepare the buffer for expected packet size
	buf.Grow(r.maxPacketSize)
	buf.Reset()
	// use Buffer's ReadFrom() as otherwise it's hard to get it into the right state
	n, err := buf.ReadFrom(r)
	return int(n), err
}

// Serve initiates the readers and starts serving traffic
func (s *TBufferedServer) Serve() {
	defer close(s.dataChan)
	if !atomic.CompareAndSwapUint32(&s.serving, stateInit, stateServing) {
		return // Stop already called
	}

	pr := &packetReader{
		maxPacketSize: s.maxPacketSize,
		reader: &io.LimitedReader{
			R: s.transport,
		},
	}

	for s.IsServing() {
		buf := s.readBufPool.Get().(*bytes.Buffer)
		n, err := pr.readPacket(buf)
		if err == nil {
			s.metrics.PacketSize.Update(int64(n))
			select {
			case s.dataChan <- buf:
				s.metrics.PacketsProcessed.Inc(1)
				s.updateQueueSize(1)
			default:
				s.readBufPool.Put(buf)
				s.metrics.PacketsDropped.Inc(1)
			}
		} else {
			s.readBufPool.Put(buf)
			s.metrics.ReadError.Inc(1)
		}
	}
}

func (s *TBufferedServer) updateQueueSize(delta int64) {
	atomic.AddInt64(&s.queueSize, delta)
	s.metrics.QueueSize.Update(atomic.LoadInt64(&s.queueSize))
}

// IsServing indicates whether the server is currently serving traffic
func (s *TBufferedServer) IsServing() bool {
	return atomic.LoadUint32(&s.serving) == stateServing
}

// Stop stops the serving of traffic and waits until the queue is
// emptied by the readers
func (s *TBufferedServer) Stop() {
	atomic.StoreUint32(&s.serving, stateStopped)
	_ = s.transport.Close()
}

// DataChan returns the data chan of the buffered server
func (s *TBufferedServer) DataChan() chan *bytes.Buffer {
	return s.dataChan
}

// DataRecd is called by the consumers every time they read a data item from DataChan
func (s *TBufferedServer) DataRecd(buf *bytes.Buffer) {
	s.updateQueueSize(-1)
	s.readBufPool.Put(buf)
}
