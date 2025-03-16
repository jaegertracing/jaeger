// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package servers

import (
	"bytes"
	"io"
	"sync"
	"sync/atomic"
)

// UDPConn is a an abstraction of *net.UDPConn, for easier mocking.
type UDPConn interface {
	io.Reader
	io.Closer
}

// UDPServerHandler is an function called by UDPServer to handle
// the received packet data. Upon completion of the processing
// the handler SHOULD call the release function to return the buffer to the pool.
type UDPServerHandler func(buf *bytes.Buffer, release func(*bytes.Buffer))

// UDPServer reads packets from a UDP connection into bytes.Buffer and places
// each buffer into a bounded channel to be consumed by the receiver.
// After consuming the buffer, the receiver SHOULD call DataRecd() to signal
// that the buffer is no longer in use and to return it to the pool.
type UDPServer struct {
	// NB. queueLength HAS to be at the top of the struct or it will SIGSEV for certain architectures.
	// See https://github.com/golang/go/issues/13868
	// queueSize     int64
	serving uint32
	handler UDPServerHandler

	// dataChan      chan *bytes.Buffer
	maxPacketSize int
	// maxQueueSize  int
	transport   UDPConn
	readBufPool sync.Pool
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

// NewUDPServer creates a UDPServer
func NewUDPServer(
	transport UDPConn,
	handler UDPServerHandler,
	maxPacketSize int,
) (*UDPServer, error) {
	return &UDPServer{
		transport:     transport,
		handler:       handler,
		maxPacketSize: maxPacketSize,
		serving:       stateInit,
		readBufPool: sync.Pool{
			New: func() any {
				return new(bytes.Buffer)
			},
		},
	}, nil
}

// packetReader is a helper for reading a single packet no larger than maxPacketSize
// from the underlying reader. Without it the ReadFrom() method of bytes.Buffer would
// read multiple packets and won't even stop at maxPacketSize.
type packetReader struct {
	maxPacketSize int
	reader        io.LimitedReader
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
func (s *UDPServer) Serve() {
	if !atomic.CompareAndSwapUint32(&s.serving, stateInit, stateServing) {
		return // Stop already called
	}

	pr := &packetReader{
		maxPacketSize: s.maxPacketSize,
		reader: io.LimitedReader{
			R: s.transport,
		},
	}

	release := func(buf *bytes.Buffer) {
		s.readBufPool.Put(buf)
	}

	for s.IsServing() {
		buf := s.readBufPool.Get().(*bytes.Buffer)
		_, err := pr.readPacket(buf)
		if err == nil {
			s.handler(buf, release)
		} else {
			s.readBufPool.Put(buf)
		}
	}
}

// IsServing indicates whether the server is currently serving traffic
func (s *UDPServer) IsServing() bool {
	return atomic.LoadUint32(&s.serving) == stateServing
}

// Stop stops the serving of traffic and waits until the queue is
// emptied by the readers
func (s *UDPServer) Stop() {
	if v := atomic.SwapUint32(&s.serving, stateStopped); v == stateStopped {
		return // already stopped
	}
	_ = s.transport.Close()
}
