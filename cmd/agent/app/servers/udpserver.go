// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package servers

import (
	"bytes"
	"io"
	"net"
	"sync"
	"sync/atomic"

	"github.com/jaegertracing/jaeger/cmd/agent/app/servers/thriftudp"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

// UDPConn is a subset of interfaces of *net.UDPConn methods, for easier mocking.
type UDPConn interface {
	io.Reader
	io.Closer
}

// UDPServer reads traffic packets from a UDP connection into bytes.Buffer
// and places them into a buffered channel to be consumed by the receiver.
type UDPServer struct {
	// NB. queueLength HAS to be at the top of the struct or it will SIGSEV for certain architectures.
	// See https://github.com/golang/go/issues/13868
	queueSize     int64
	dataChan      chan *bytes.Buffer
	maxPacketSize int
	maxQueueSize  int
	serving       uint32
	conn          UDPConn
	addr          net.Addr
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

// state values for UDPServer.serving
//
// init -> serving -> stopped
// init -> stopped (might happen in unit tests)
const (
	stateStopped = iota
	stateServing
	stateInit
)

// NewUDPServer creates a UDPServer
func NewUDPServer(
	hostPort string,
	maxQueueSize int,
	maxPacketSize int,
	mFactory metrics.Factory,
) (*UDPServer, error) {
	addr, err := net.ResolveUDPAddr("udp", hostPort)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP(addr.Network(), addr)
	if err != nil {
		return nil, err
	}
	if err := thriftudp.SetSocketBuffer(conn, maxPacketSize); err != nil {
		return nil, err
	}

	res := &UDPServer{
		dataChan:      make(chan *bytes.Buffer, maxQueueSize),
		conn:          conn,
		addr:          conn.LocalAddr(),
		maxQueueSize:  maxQueueSize,
		maxPacketSize: maxPacketSize,
		serving:       stateInit,
		readBufPool: sync.Pool{
			New: func() any {
				b := new(bytes.Buffer)
				return b
			},
		},
	}

	metrics.MustInit(&res.metrics, mFactory, nil)
	return res, nil
}

func (s *UDPServer) Addr() net.Addr {
	return s.addr
}

func (s *UDPServer) readPacket(buf *bytes.Buffer) (int, error) {
	buf.Grow(s.maxPacketSize)
	buf.Reset()
	n, err := s.conn.Read(buf.Bytes())
	if err != nil {
		return 0, err
	}
	buf.Truncate(n)
	return n, nil
}

// Serve initiates the readers and starts serving traffic
func (s *UDPServer) Serve() {
	defer close(s.dataChan)
	if !atomic.CompareAndSwapUint32(&s.serving, stateInit, stateServing) {
		return // Stop already called
	}

	for s.IsServing() {
		buf := s.readBufPool.Get().(*bytes.Buffer)
		n, err := s.readPacket(buf)
		if err == nil && n > 0 {
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

func (s *UDPServer) updateQueueSize(delta int64) {
	atomic.AddInt64(&s.queueSize, delta)
	s.metrics.QueueSize.Update(atomic.LoadInt64(&s.queueSize))
}

// IsServing indicates whether the server is currently serving traffic
func (s *UDPServer) IsServing() bool {
	return atomic.LoadUint32(&s.serving) == stateServing
}

// Stop stops the serving of traffic and waits until the queue is
// emptied by the readers
func (s *UDPServer) Stop() {
	atomic.StoreUint32(&s.serving, stateStopped)
	_ = s.conn.Close()
}

// DataChan returns the data chan of the buffered server
func (s *UDPServer) DataChan() chan *bytes.Buffer {
	return s.dataChan
}

// DataRecd is called by the consumers every time they read a data item from DataChan
func (s *UDPServer) DataRecd(buf *bytes.Buffer) {
	s.updateQueueSize(-1)
	s.readBufPool.Put(buf)
}
