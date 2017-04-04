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
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/uber/jaeger-lib/metrics"
	mTestutils "github.com/uber/jaeger-lib/metrics/testutils"
	"github.com/uber/jaeger/thrift-gen/agent"
	"github.com/uber/jaeger/thrift-gen/jaeger"
	"github.com/uber/jaeger/thrift-gen/zipkincore"

	"github.com/uber/jaeger/cmd/agent/app/reporter"
	"github.com/uber/jaeger/cmd/agent/app/servers"
	"github.com/uber/jaeger/cmd/agent/app/servers/thriftudp"
	"github.com/uber/jaeger/cmd/agent/app/testutils"
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

	queueSize := 10
	maxPacketSize := 65000
	server, err := servers.NewTBufferedServer(transport, queueSize, maxPacketSize, mFactory)
	require.NoError(t, err)

	numProcessors := 1
	processor, err := NewThriftProcessor(server, numProcessors, mFactory, tFactory, handler)
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

func initCollectorAndReporter(t *testing.T) (*metrics.LocalFactory, *testutils.MockTCollector, reporter.Reporter) {
	metricsFactory, collector := testutils.InitMockCollector(t)
	reporter := reporter.NewTChannelReporter("jaeger-collector", collector.Channel, metricsFactory, zap.NewNop(), nil)
	return metricsFactory, collector, reporter
}

func TestNewThriftProcessor_ZeroCount(t *testing.T) {
	_, err := NewThriftProcessor(nil, 0, nil, nil, nil)
	assert.EqualError(t, err, "Number of processors must be greater than 0, called with 0")
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
	metricsFactory := metrics.NewLocalFactory(0)

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

	mTestutils.AssertCounterMetrics(t, metricsFactory,
		mTestutils.ExpectedMetric{Name: "thrift.udp.t-processor.handler-errors", Value: 1},
		mTestutils.ExpectedMetric{Name: "thrift.udp.server.packets.processed", Value: 1},
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

func assertJaegerProcessorCorrectness(t *testing.T, collector *testutils.MockTCollector, metricsFactory *metrics.LocalFactory) {
	sizeF := func() int {
		return len(collector.GetJaegerBatches())
	}
	nameF := func() string {
		return collector.GetJaegerBatches()[0].Spans[0].OperationName
	}
	assertProcessorCorrectness(t, metricsFactory, sizeF, nameF, "jaeger")
}

func assertZipkinProcessorCorrectness(t *testing.T, collector *testutils.MockTCollector, metricsFactory *metrics.LocalFactory) {
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
	metricsFactory *metrics.LocalFactory,
	sizeF func() int,
	nameF func() string,
	counterType string,
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
	batchesSubmittedCounter := fmt.Sprintf("tc-reporter.%s.batches.submitted", counterType)
	spansSubmittedCounter := fmt.Sprintf("tc-reporter.%s.spans.submitted", counterType)
	mTestutils.AssertCounterMetrics(t, metricsFactory, []mTestutils.ExpectedMetric{
		{Name: batchesSubmittedCounter, Value: 1},
		{Name: spansSubmittedCounter, Value: 1},
		{Name: "thrift.udp.server.packets.processed", Value: 1},
	}...)
}
