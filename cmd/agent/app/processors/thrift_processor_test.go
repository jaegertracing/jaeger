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
	"errors"
	"testing"
	"time"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger-lib/metrics/metricstest"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
	tchreporter "github.com/jaegertracing/jaeger/cmd/agent/app/reporter/tchannel"
	"github.com/jaegertracing/jaeger/cmd/agent/app/servers"
	"github.com/jaegertracing/jaeger/cmd/agent/app/servers/thriftudp"
	"github.com/jaegertracing/jaeger/cmd/agent/app/testutils"
	"github.com/jaegertracing/jaeger/thrift-gen/agent"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

// TODO make these tests faster, they take almost 4 seconds

var (
	compactFactory = thrift.NewTCompactProtocolFactory()
	binaryFactory  = thrift.NewTBinaryProtocolFactoryDefault()

	testSpanName = "span1"

	batch = &jaeger.Batch{
		Process: jaeger.NewProcess(),
		Spans:   []*jaeger.Span{{OperationName: testSpanName}},
	}
)

func createProcessor(t *testing.T, mFactory metrics.Factory, tFactory thrift.TProtocolFactory, handler AgentProcessor) (string, Processor) {
	transport, err := thriftudp.NewTUDPServerTransport("127.0.0.1:0")
	require.NoError(t, err)

	maxPacketSize := 65000
	server, err := servers.NewTBufferedServer(transport, maxPacketSize, mFactory)
	require.NoError(t, err)

	processor, err := NewThriftProcessor(server, mFactory, tFactory, handler, zap.NewNop())
	require.NoError(t, err)

	go processor.Serve()
	for i := 0; i < 1000; i++ {
		if processor.IsServing() {
			break
		}
		time.Sleep(10 * time.Microsecond)
	}
	require.True(t, processor.IsServing(), "processor must be serving")

	return transport.Addr().String(), processor
}

func initCollectorAndReporter(t *testing.T) (*metricstest.Factory, *testutils.MockTCollector, reporter.Reporter) {
	metricsFactory, collector := testutils.InitMockCollector(t)

	reporter := reporter.NewMetricsReporter(tchreporter.New("jaeger-collector", collector.Channel, time.Second, nil, zap.NewNop()), metricsFactory)

	return metricsFactory, collector, reporter
}

func TestProcessorWithCompactZipkin(t *testing.T) {
	metricsFactory, collector, reporter := initCollectorAndReporter(t)
	defer collector.Close()

	hostPort, processor := createProcessor(t, metricsFactory, compactFactory, agent.NewAgentProcessor(reporter))
	defer processor.Stop()

	client, clientCloser, err := testutils.NewZipkinThriftUDPClient(hostPort)
	require.NoError(t, err)
	defer clientCloser.Close()

	span := zipkincore.NewSpan()
	span.Name = testSpanName

	err = client.EmitZipkinBatch([]*zipkincore.Span{span})
	require.NoError(t, err)

	assertZipkinProcessorCorrectness(t, collector, metricsFactory)
}

type failingHandler struct {
	err error
}

func (h failingHandler) Process(iprot, oprot thrift.TProtocol) (success bool, err thrift.TException) {
	return false, thrift.NewTApplicationException(0, h.err.Error())
}

func TestProcessor_HandlerError(t *testing.T) {
	metricsFactory := metricstest.NewFactory(0)

	handler := failingHandler{err: errors.New("doh")}

	hostPort, processor := createProcessor(t, metricsFactory, compactFactory, handler)
	defer processor.Stop()

	client, clientCloser, err := testutils.NewZipkinThriftUDPClient(hostPort)
	require.NoError(t, err)
	defer clientCloser.Close()

	err = client.EmitZipkinBatch([]*zipkincore.Span{{Name: testSpanName}})
	require.NoError(t, err)

	for i := 0; i < 10; i++ {
		c, _ := metricsFactory.Snapshot()
		if _, ok := c["thrift.udp.t-processor.handler-errors"]; ok {
			break
		}
		time.Sleep(time.Millisecond)
	}

	metricsFactory.AssertCounterMetrics(t,
		metricstest.ExpectedMetric{Name: "thrift.udp.t-processor.handler-errors", Value: 1},
		metricstest.ExpectedMetric{Name: "thrift.udp.server.packets.processed", Value: 1},
	)
}

func TestJaegerProcessor(t *testing.T) {
	tests := []struct {
		factory thrift.TProtocolFactory
	}{
		{compactFactory},
		{binaryFactory},
	}

	for _, test := range tests {
		metricsFactory, collector, reporter := initCollectorAndReporter(t)

		hostPort, processor := createProcessor(t, metricsFactory, test.factory, jaeger.NewAgentProcessor(reporter))

		client, clientCloser, err := testutils.NewJaegerThriftUDPClient(hostPort, test.factory)
		require.NoError(t, err)

		err = client.EmitBatch(batch)
		require.NoError(t, err)

		assertJaegerProcessorCorrectness(t, collector, metricsFactory)

		processor.Stop()
		clientCloser.Close()
		collector.Close()
	}
}

func assertJaegerProcessorCorrectness(t *testing.T, collector *testutils.MockTCollector, metricsFactory *metricstest.Factory) {
	sizeF := func() int {
		return len(collector.GetJaegerBatches())
	}
	nameF := func() string {
		return collector.GetJaegerBatches()[0].Spans[0].OperationName
	}
	assertProcessorCorrectness(t, metricsFactory, sizeF, nameF, "jaeger")
}

func assertZipkinProcessorCorrectness(t *testing.T, collector *testutils.MockTCollector, metricsFactory *metricstest.Factory) {
	sizeF := func() int {
		return len(collector.GetZipkinSpans())
	}
	nameF := func() string {
		return collector.GetZipkinSpans()[0].Name
	}
	assertProcessorCorrectness(t, metricsFactory, sizeF, nameF, "zipkin")
}

func assertProcessorCorrectness(
	t *testing.T,
	metricsFactory *metricstest.Factory,
	sizeF func() int,
	nameF func() string,
	format string,
) {
	// wait for server to receive
	for i := 0; i < 1000; i++ {
		if sizeF() == 1 {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}

	require.Equal(t, 1, sizeF())
	assert.Equal(t, testSpanName, nameF())

	// wait for reporter to emit metrics
	for i := 0; i < 1000; i++ {
		c, _ := metricsFactory.Snapshot()
		if _, ok := c["tc-reporter.spans.submitted"]; ok {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}

	// agentReporter must emit metrics
	metricsFactory.AssertCounterMetrics(t, []metricstest.ExpectedMetric{
		{Name: "reporter.batches.submitted", Tags: map[string]string{"format": format}, Value: 1},
		{Name: "reporter.spans.submitted", Tags: map[string]string{"format": format}, Value: 1},
		{Name: "thrift.udp.server.packets.processed", Value: 1},
	}...)
}
