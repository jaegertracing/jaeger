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
	"context"
	"errors"
	"testing"
	"time"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
	grpcrep "github.com/jaegertracing/jaeger/cmd/agent/app/reporter/grpc"
	"github.com/jaegertracing/jaeger/cmd/agent/app/servers"
	"github.com/jaegertracing/jaeger/cmd/agent/app/servers/thriftudp"
	"github.com/jaegertracing/jaeger/cmd/agent/app/testutils"
	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/thrift-gen/agent"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

// TODO make these tests faster, they take almost 4 seconds

var (
	compactFactory = thrift.NewTCompactProtocolFactoryConf(&thrift.TConfiguration{})
	binaryFactory  = thrift.NewTBinaryProtocolFactoryConf(&thrift.TConfiguration{})

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
	processor, err := NewThriftProcessor(server, numProcessors, mFactory, tFactory, handler, zap.NewNop())
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

func initCollectorAndReporter(t *testing.T) (*metricstest.Factory, *testutils.GrpcCollector, reporter.Reporter, *grpc.ClientConn) {
	grpcCollector := testutils.StartGRPCCollector(t)
	conn, err := grpc.Dial(grpcCollector.Listener().Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	rep := grpcrep.NewReporter(conn, map[string]string{}, zap.NewNop())
	metricsFactory := metricstest.NewFactory(0)
	reporter := reporter.WrapWithMetrics(rep, metricsFactory)
	return metricsFactory, grpcCollector, reporter, conn
}

func TestNewThriftProcessor_ZeroCount(t *testing.T) {
	_, err := NewThriftProcessor(nil, 0, nil, nil, nil, zap.NewNop())
	assert.EqualError(t, err, "number of processors must be greater than 0, called with 0")
}

func TestProcessorWithCompactZipkin(t *testing.T) {
	metricsFactory, collector, reporter, conn := initCollectorAndReporter(t)
	defer conn.Close()
	defer collector.Close()

	hostPort, processor := createProcessor(t, metricsFactory, compactFactory, agent.NewAgentProcessor(reporter))
	defer processor.Stop()

	client, clientCloser, err := testutils.NewZipkinThriftUDPClient(hostPort)
	require.NoError(t, err)
	defer clientCloser.Close()

	span := zipkincore.NewSpan()
	span.Name = testSpanName
	span.Annotations = []*zipkincore.Annotation{{Value: zipkincore.CLIENT_SEND, Host: &zipkincore.Endpoint{ServiceName: "foo"}}}

	err = client.EmitZipkinBatch(context.Background(), []*zipkincore.Span{span})
	require.NoError(t, err)

	assertZipkinProcessorCorrectness(t, collector, metricsFactory)
}

type failingHandler struct {
	err error
}

func (h failingHandler) Process(_ context.Context, iprot, oprot thrift.TProtocol) (success bool, err thrift.TException) {
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

	err = client.EmitZipkinBatch(context.Background(), []*zipkincore.Span{{Name: testSpanName}})
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
		metricsFactory, collector, reporter, conn := initCollectorAndReporter(t)

		hostPort, processor := createProcessor(t, metricsFactory, test.factory, agent.NewAgentProcessor(reporter))

		client, clientCloser, err := testutils.NewJaegerThriftUDPClient(hostPort, test.factory)
		require.NoError(t, err)

		err = client.EmitBatch(context.Background(), batch)
		require.NoError(t, err)

		assertJaegerProcessorCorrectness(t, collector, metricsFactory)

		processor.Stop()
		clientCloser.Close()
		conn.Close()
		collector.Close()
	}
}

func assertJaegerProcessorCorrectness(t *testing.T, collector *testutils.GrpcCollector, metricsFactory *metricstest.Factory) {
	sizeF := func() int {
		return len(collector.GetJaegerBatches())
	}
	nameF := func() string {
		return collector.GetJaegerBatches()[0].Spans[0].OperationName
	}
	assertProcessorCorrectness(t, metricsFactory, sizeF, nameF, "jaeger")
}

func assertZipkinProcessorCorrectness(t *testing.T, collector *testutils.GrpcCollector, metricsFactory *metricstest.Factory) {
	sizeF := func() int {
		return len(collector.GetJaegerBatches())
	}
	nameF := func() string {
		return collector.GetJaegerBatches()[0].Spans[0].OperationName
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
