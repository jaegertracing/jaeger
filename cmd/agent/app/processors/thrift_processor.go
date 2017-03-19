// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package processors

import (
	"fmt"
	"sync"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/uber/jaeger-lib/metrics"

	"github.com/uber/jaeger/cmd/agent/app/customtransports"
	"github.com/uber/jaeger/cmd/agent/app/servers"
)

// ThriftProcessor is a server that processes spans using a TBuffered Server
type ThriftProcessor struct {
	server        servers.Server
	handler       AgentProcessor
	protocolPool  *sync.Pool
	numProcessors int
	processing    sync.WaitGroup
	metrics       struct {
		// Amount of time taken for processor to close
		ProcessorCloseTimer metrics.Timer `metric:"thrift.udp.t-processor.close-time"`

		// Number of failed buffer process operations
		HandlerProcessError metrics.Counter `metric:"thrift.udp.t-processor.handler-errors"`
	}
}

// AgentProcessor handler used by the processor to process thrift and call the reporter with the deserialized struct
type AgentProcessor interface {
	Process(iprot, oprot thrift.TProtocol) (success bool, err thrift.TException)
}

// NewThriftProcessor creates a TBufferedServer backed ThriftProcessor
func NewThriftProcessor(
	server servers.Server,
	numProcessors int,
	mFactory metrics.Factory,
	factory thrift.TProtocolFactory,
	handler AgentProcessor,
) (*ThriftProcessor, error) {
	if numProcessors <= 0 {
		return nil, fmt.Errorf(
			"Number of processors must be greater than 0, called with %d", numProcessors)
	}
	var protocolPool = &sync.Pool{
		New: func() interface{} {
			trans := &customtransport.TBufferedReadTransport{}
			return factory.GetProtocol(trans)
		},
	}

	res := &ThriftProcessor{
		server:        server,
		handler:       handler,
		protocolPool:  protocolPool,
		numProcessors: numProcessors,
	}
	metrics.Init(&res.metrics, mFactory, nil)
	return res, nil
}

// Serve initiates the readers and starts serving traffic
func (s *ThriftProcessor) Serve() {
	s.processing.Add(s.numProcessors)
	for i := 0; i < s.numProcessors; i++ {
		go s.processBuffer()
	}

	s.server.Serve()
}

// IsServing indicates whether the server is currently serving traffic
func (s *ThriftProcessor) IsServing() bool {
	return s.server.IsServing()
}

// Stop stops the serving of traffic and waits until the queue is
// emptied by the readers
func (s *ThriftProcessor) Stop() {
	stopwatch := metrics.StartStopwatch(s.metrics.ProcessorCloseTimer)
	s.server.Stop()
	s.processing.Wait()
	stopwatch.Stop()
}

// processBuffer reads data off the channel and puts it into a custom transport for
// the processor to process
func (s *ThriftProcessor) processBuffer() {
	for readBuf := range s.server.DataChan() {
		protocol := s.protocolPool.Get().(thrift.TProtocol)
		protocol.Transport().Write(readBuf.GetBytes())
		s.server.DataRecd(readBuf) // acknowledge receipt and release the buffer

		if ok, _ := s.handler.Process(protocol, protocol); !ok {
			// TODO log the error
			s.metrics.HandlerProcessError.Inc(1)
		}
		s.protocolPool.Put(protocol)
	}
	s.processing.Done()
}
