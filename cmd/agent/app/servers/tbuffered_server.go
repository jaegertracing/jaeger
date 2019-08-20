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
	"sync/atomic"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/uber/jaeger-lib/metrics"
)

// TBufferedServer is a custom thrift server that reads traffic using the transport provided
// and places messages into a buffered channel to be processed by the processor provided
type TBufferedServer struct {
	// NB. queueLength HAS to be at the top of the struct or it will SIGSEV for certain architectures.
	// See https://github.com/golang/go/issues/13868
	queueSize     int64
	processor     func(*ReadBuf)
	maxPacketSize int
	serving       uint32
	transport     thrift.TTransport
	readBufPool   *sync.Pool
	metrics       struct {
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
	transport thrift.TTransport,
	maxPacketSize int,
	mFactory metrics.Factory,
) (*TBufferedServer, error) {
	var readBufPool = &sync.Pool{
		New: func() interface{} {
			return &ReadBuf{bytes: make([]byte, maxPacketSize)}
		},
	}

	res := &TBufferedServer{
		transport:     transport,
		maxPacketSize: maxPacketSize,
		readBufPool:   readBufPool,
	}
	metrics.Init(&res.metrics, mFactory, nil)
	return res, nil
}

// Serve initiates the readers and starts serving traffic
func (s *TBufferedServer) Serve() {
	if s.processor == nil {
		panic("Invalid configuration for TBufferedServer, no processor defined")
	}
	atomic.StoreUint32(&s.serving, 1)
	for s.IsServing() {
		readBuf := s.readBufPool.Get().(*ReadBuf)
		n, err := s.transport.Read(readBuf.bytes)
		if err == nil {
			readBuf.n = n
			s.metrics.PacketSize.Update(int64(n))
			go func() {
				s.processor(readBuf)
				s.readBufPool.Put(readBuf)
			}()
		} else {
			s.metrics.ReadError.Inc(1)
		}
	}
}

// IsServing indicates whether the server is currently serving traffic
func (s *TBufferedServer) IsServing() bool {
	return atomic.LoadUint32(&s.serving) == 1
}

// Stop stops the serving of traffic and waits until the queue is
// emptied by the readers
func (s *TBufferedServer) Stop() {
	atomic.StoreUint32(&s.serving, 0)
	s.transport.Close()
}

func (s *TBufferedServer) RegisterProcessor(process func(*ReadBuf)) {
	s.processor = process
}
