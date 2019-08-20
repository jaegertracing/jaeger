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

package processors

import (
	"sync"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	customtransport "github.com/jaegertracing/jaeger/cmd/agent/app/customtransports"
	"github.com/jaegertracing/jaeger/cmd/agent/app/servers"
)

// ThriftProcessor is a server that processes spans using a TBuffered Server
type ThriftProcessor struct {
	server       servers.Server
	handler      AgentProcessor
	protocolPool *sync.Pool
	logger       *zap.Logger
	processing   sync.WaitGroup
	metrics      struct {
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
	mFactory metrics.Factory,
	factory thrift.TProtocolFactory,
	handler AgentProcessor,
	logger *zap.Logger,
) (*ThriftProcessor, error) {
	var protocolPool = &sync.Pool{
		New: func() interface{} {
			trans := &customtransport.TBufferedReadTransport{}
			return factory.GetProtocol(trans)
		},
	}

	res := &ThriftProcessor{
		server:       server,
		handler:      handler,
		protocolPool: protocolPool,
		logger:       logger,
	}
	metrics.Init(&res.metrics, mFactory, nil)
	return res, nil
}

// Serve starts serving traffic
func (s *ThriftProcessor) Serve() {
	s.server.RegisterProcessor(s.processBuffer)
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

// processBuffer reads data off the buffer and puts it into a custom transport for
// the processor to process
func (s *ThriftProcessor) processBuffer(readBuf *servers.ReadBuf) {
	s.processing.Add(1)
	defer s.processing.Done()

	protocol := s.protocolPool.Get().(thrift.TProtocol)
	payload := readBuf.GetBytes()
	protocol.Transport().Write(payload)
	s.logger.Debug("Span(s) received by the agent", zap.Int("bytes-received", len(payload)))

	if ok, err := s.handler.Process(protocol, protocol); !ok {
		s.logger.Error("Processor failed", zap.Error(err))
		s.metrics.HandlerProcessError.Inc(1)
	}
	s.protocolPool.Put(protocol)
}
