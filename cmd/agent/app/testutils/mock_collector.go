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

package testutils

import (
	"errors"
	"sync"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"

	"github.com/jaegertracing/jaeger/thrift-gen/baggage"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

// StartMockTCollector runs a mock representation of Jaeger Collector.
// This function returns a started server, with a Channel that knows
// how to find that server, which can be used in clients or Jaeger tracer.
//
// TODO we should refactor normal collector so it can be used in tests, with in-memory storage
func StartMockTCollector() (*MockTCollector, error) {
	return startMockTCollector("jaeger-collector", "127.0.0.1:0")
}

// extracted to separate function to be able to test error cases
func startMockTCollector(name string, addr string) (*MockTCollector, error) {
	ch, err := tchannel.NewChannel(name, nil)
	if err != nil {
		return nil, err
	}

	server := thrift.NewServer(ch)

	collector := &MockTCollector{
		Channel:               ch,
		server:                server,
		zipkinSpans:           make([]*zipkincore.Span, 0, 10),
		jaegerBatches:         make([]*jaeger.Batch, 0, 10),
		samplingMgr:           newSamplingManager(),
		baggageRestrictionMgr: newBaggageRestrictionManager(),
		ReturnErr:             false,
	}

	server.Register(zipkincore.NewTChanZipkinCollectorServer(collector))
	server.Register(jaeger.NewTChanCollectorServer(collector))
	server.Register(sampling.NewTChanSamplingManagerServer(&tchanSamplingManager{collector.samplingMgr}))
	server.Register(baggage.NewTChanBaggageRestrictionManagerServer(&tchanBaggageRestrictionManager{collector.baggageRestrictionMgr}))

	if err := ch.ListenAndServe(addr); err != nil {
		return nil, err
	}

	subchannel := ch.GetSubChannel("jaeger-collector", tchannel.Isolated)
	subchannel.Peers().Add(ch.PeerInfo().HostPort)

	return collector, nil
}

// MockTCollector is a mock representation of Jaeger Collector.
type MockTCollector struct {
	Channel               *tchannel.Channel
	server                *thrift.Server
	zipkinSpans           []*zipkincore.Span
	jaegerBatches         []*jaeger.Batch
	mutex                 sync.Mutex
	samplingMgr           *samplingManager
	baggageRestrictionMgr *baggageRestrictionManager
	ReturnErr             bool
}

// AddSamplingStrategy registers a sampling strategy for a service
func (s *MockTCollector) AddSamplingStrategy(
	service string,
	strategy *sampling.SamplingStrategyResponse,
) {
	s.samplingMgr.AddSamplingStrategy(service, strategy)
}

// AddBaggageRestrictions registers baggage restrictions for a service
func (s *MockTCollector) AddBaggageRestrictions(
	service string,
	restrictions []*baggage.BaggageRestriction,
) {
	s.baggageRestrictionMgr.AddBaggageRestrictions(service, restrictions)
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

type tchanBaggageRestrictionManager struct {
	baggageRestrictionManager *baggageRestrictionManager
}

// GetBaggageRestrictions implements GetBaggageRestrictions of TChanBaggageRestrictionManager
func (m *tchanBaggageRestrictionManager) GetBaggageRestrictions(
	ctx thrift.Context,
	serviceName string,
) ([]*baggage.BaggageRestriction, error) {
	return m.baggageRestrictionManager.GetBaggageRestrictions(serviceName)
}
