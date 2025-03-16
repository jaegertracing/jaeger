// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package servers

import (
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
	dataChan      chan *ReadBuf
	maxPacketSize int
	maxQueueSize  int
	serving       uint32
	transport     ThriftTransport
	readBufPool   *sync.Pool
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

// NewTBufferedServer creates a TBufferedServer
func NewTBufferedServer(
	transport ThriftTransport,
	maxQueueSize int,
	maxPacketSize int,
	mFactory metrics.Factory,
) (*TBufferedServer, error) {
	dataChan := make(chan *ReadBuf, maxQueueSize)

	readBufPool := &sync.Pool{
		New: func() any {
			return &ReadBuf{bytes: make([]byte, maxPacketSize)}
		},
	}

	res := &TBufferedServer{
		dataChan:      dataChan,
		transport:     transport,
		maxQueueSize:  maxQueueSize,
		maxPacketSize: maxPacketSize,
		readBufPool:   readBufPool,
		serving:       stateInit,
	}

	metrics.MustInit(&res.metrics, mFactory, nil)
	return res, nil
}

// Serve initiates the readers and starts serving traffic
func (s *TBufferedServer) Serve() {
	defer close(s.dataChan)
	if !atomic.CompareAndSwapUint32(&s.serving, stateInit, stateServing) {
		return // Stop already called
	}

	for s.IsServing() {
		readBuf := s.readBufPool.Get().(*ReadBuf)
		n, err := s.transport.Read(readBuf.bytes)
		if err == nil {
			readBuf.n = n
			s.metrics.PacketSize.Update(int64(n))
			select {
			case s.dataChan <- readBuf:
				s.metrics.PacketsProcessed.Inc(1)
				s.updateQueueSize(1)
			default:
				s.readBufPool.Put(readBuf)
				s.metrics.PacketsDropped.Inc(1)
			}
		} else {
			s.readBufPool.Put(readBuf)
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
func (s *TBufferedServer) DataChan() chan *ReadBuf {
	return s.dataChan
}

// DataRecd is called by the consumers every time they read a data item from DataChan
func (s *TBufferedServer) DataRecd(buf *ReadBuf) {
	s.updateQueueSize(-1)
	s.readBufPool.Put(buf)
}
