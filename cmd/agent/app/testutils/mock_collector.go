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

package testutils

import (
	"errors"
	"sync"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"

	"github.com/uber/jaeger/thrift-gen/jaeger"
	"github.com/uber/jaeger/thrift-gen/sampling"
	"github.com/uber/jaeger/thrift-gen/zipkincore"
)

// StartMockTCollector runs a mock representation of Jaeger Collector.
// This function returns a started server, with a Channel that knows
// how to find that server, which can be used in clients or Jaeger tracer.
//
// TODO we should refactor normal collector so it can be used in tests, with in-memory storage
func StartMockTCollector() (*MockTCollector, error) {
	return startMockTCollector("tcollector", "127.0.0.1:0")
}

// extracted to separate function to be able to test error cases
func startMockTCollector(name string, addr string) (*MockTCollector, error) {
	ch, err := tchannel.NewChannel(name, nil)
	if err != nil {
		return nil, err
	}

	server := thrift.NewServer(ch)

	collector := &MockTCollector{
		Channel:       ch,
		server:        server,
		zipkinSpans:   make([]*zipkincore.Span, 0, 10),
		jaegerBatches: make([]*jaeger.Batch, 0, 10),
		samplingMgr:   newSamplingManager(),
		ReturnErr:     false,
	}

	server.Register(zipkincore.NewTChanZipkinCollectorServer(collector))
	server.Register(jaeger.NewTChanCollectorServer(collector))
	server.Register(sampling.NewTChanSamplingManagerServer(&tchanSamplingManager{collector.samplingMgr}))

	if err := ch.ListenAndServe(addr); err != nil {
		return nil, err
	}

	subchannel := ch.GetSubChannel("tcollector", tchannel.Isolated)
	subchannel.Peers().Add(ch.PeerInfo().HostPort)

	return collector, nil
}

// MockTCollector is a mock representation of Jaeger Collector.
type MockTCollector struct {
	Channel       *tchannel.Channel
	server        *thrift.Server
	zipkinSpans   []*zipkincore.Span
	jaegerBatches []*jaeger.Batch
	mutex         sync.Mutex
	samplingMgr   *samplingManager
	ReturnErr     bool
}

// AddSamplingStrategy registers a sampling strategy for a service
func (s *MockTCollector) AddSamplingStrategy(
	service string,
	strategy *sampling.SamplingStrategyResponse,
) {
	s.samplingMgr.AddSamplingStrategy(service, strategy)
}

// GetZipkinSpans returns accumulated Zipkin spans
func (s *MockTCollector) GetZipkinSpans() []*zipkincore.Span {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.zipkinSpans[:]
}

// GetJaegerBatches returns accumulated Jaeger batches
func (s *MockTCollector) GetJaegerBatches() []*jaeger.Batch {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.jaegerBatches[:]
}

// Close stops/closes the underlying channel and server
func (s *MockTCollector) Close() {
	s.Channel.Close()
}

// SubmitZipkinBatch implements handler method of TChanZipkinCollectorServer
func (s *MockTCollector) SubmitZipkinBatch(
	ctx thrift.Context,
	spans []*zipkincore.Span,
) ([]*zipkincore.Response, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.ReturnErr {
		return []*zipkincore.Response{{Ok: false}}, errors.New("Returning error from MockTCollector")
	}
	s.zipkinSpans = append(s.zipkinSpans, spans...)
	return []*zipkincore.Response{{Ok: true}}, nil
}

// SubmitBatches implements handler method of TChanCollectorServer
func (s *MockTCollector) SubmitBatches(
	ctx thrift.Context,
	batches []*jaeger.Batch,
) ([]*jaeger.BatchSubmitResponse, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.ReturnErr {
		return []*jaeger.BatchSubmitResponse{{Ok: false}}, errors.New("Returning error from MockTCollector")
	}
	s.jaegerBatches = append(s.jaegerBatches, batches...)
	return []*jaeger.BatchSubmitResponse{{Ok: true}}, nil
}

type tchanSamplingManager struct {
	samplingMgr *samplingManager
}

// GetSamplingStrategy implements GetSamplingStrategy of TChanSamplingManagerServer
func (s *tchanSamplingManager) GetSamplingStrategy(
	ctx thrift.Context,
	serviceName string,
) (*sampling.SamplingStrategyResponse, error) {
	return s.samplingMgr.GetSamplingStrategy(serviceName)
}
