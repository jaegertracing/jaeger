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
	dataChan      chan *ReadBuf
	maxPacketSize int
	maxQueueSize  int
	serving       uint32
	transport     thrift.TTransport
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
	transport thrift.TTransport,
	maxQueueSize int,
	maxPacketSize int,
	mFactory metrics.Factory,
) (*TBufferedServer, error) {
	dataChan := make(chan *ReadBuf, maxQueueSize)

	var readBufPool = &sync.Pool{
		New: func() interface{} {
			return &ReadBuf{bytes: make([]byte, maxPacketSize)}
		},
	}

	res := &TBufferedServer{dataChan: dataChan,
		transport:     transport,
		maxQueueSize:  maxQueueSize,
		maxPacketSize: maxPacketSize,
		readBufPool:   readBufPool,
	}
	metrics.Init(&res.metrics, mFactory, nil)
	return res, nil
}

// Serve initiates the readers and starts serving traffic
func (s *TBufferedServer) Serve() {
	atomic.StoreUint32(&s.serving, 1)
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
				s.metrics.PacketsDropped.Inc(1)
			}
		} else {
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
	return atomic.LoadUint32(&s.serving) == 1
}

// Stop stops the serving of traffic and waits until the queue is
// emptied by the readers
func (s *TBufferedServer) Stop() {
	atomic.StoreUint32(&s.serving, 0)
	s.transport.Close()
	close(s.dataChan)
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
