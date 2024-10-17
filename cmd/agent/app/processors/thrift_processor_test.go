// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package processors

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
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
	t.Helper()
	transport, err := thriftudp.NewTUDPServerTransport("127.0.0.1:0")
	require.NoError(t, err)

	queueSize := 10
	maxPacketSize := 65000
	server, err := servers.NewTBufferedServer(transport, queueSize, maxPacketSize, mFactory)
	require.NoError(t, err)

	numProcessors := 1
	processor, err := NewThriftProcessor(server, numProcessors, mFactory, tFactory, handler, zaptest.NewLogger(t))
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

// revive:disable-next-line function-result-limit
func initCollectorAndReporter(t *testing.T) (*metricstest.Factory, *testutils.GrpcCollector, reporter.Reporter, *grpc.ClientConn) {
	t.Helper()
	grpcCollector := testutils.StartGRPCCollector(t)
	conn, err := grpc.NewClient(grpcCollector.Listener().Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	rep := grpcrep.NewReporter(conn, map[string]string{}, zaptest.NewLogger(t))
	metricsFactory := metricstest.NewFactory(0)
	reporter := reporter.WrapWithMetrics(rep, metricsFactory)
	return metricsFactory, grpcCollector, reporter, conn
}

func TestNewThriftProcessor_ZeroCount(t *testing.T) {
	_, err := NewThriftProcessor(nil, 0, nil, nil, nil, zaptest.NewLogger(t))
	require.EqualError(t, err, "number of processors must be greater than 0, called with 0")
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

func (h failingHandler) Process(context.Context, thrift.TProtocol /* iprot */, thrift.TProtocol /* oprot */) (success bool, err thrift.TException) {
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

// TestJaegerProcessor instantiates a real UDP receiver and a real gRPC collector
// and executes end-to-end batch submission.
func TestJaegerProcessor(t *testing.T) {
	tests := []struct {
		factory thrift.TProtocolFactory
		name    string
	}{
		{compactFactory, "compact"},
		{binaryFactory, "binary"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
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
		})
	}
}

func assertJaegerProcessorCorrectness(t *testing.T, collector *testutils.GrpcCollector, metricsFactory *metricstest.Factory) {
	t.Helper()
	sizeF := func() int {
		return len(collector.GetJaegerBatches())
	}
	nameF := func() string {
		return collector.GetJaegerBatches()[0].Spans[0].OperationName
	}
	assertCollectorReceivedData(t, metricsFactory, sizeF, nameF, "jaeger")
}

func assertZipkinProcessorCorrectness(t *testing.T, collector *testutils.GrpcCollector, metricsFactory *metricstest.Factory) {
	t.Helper()
	sizeF := func() int {
		return len(collector.GetJaegerBatches())
	}
	nameF := func() string {
		return collector.GetJaegerBatches()[0].Spans[0].OperationName
	}
	assertCollectorReceivedData(t, metricsFactory, sizeF, nameF, "zipkin")
}

// assertCollectorReceivedData verifies that collector received the data
// and that agent reporter emitted corresponding metrics.
func assertCollectorReceivedData(
	t *testing.T,
	metricsFactory *metricstest.Factory,
	sizeF func() int,
	nameF func() string,
	format string,
) {
	t.Helper()
	require.Eventually(t,
		func() bool { return sizeF() == 1 },
		5*time.Second,
		time.Millisecond,
		"server should have received spans")
	assert.Equal(t, testSpanName, nameF())

	key := "reporter.spans.submitted|format=" + format
	ok := assert.Eventuallyf(t,
		func() bool {
			c, _ := metricsFactory.Snapshot()
			_, ok := c[key]
			return ok
		},
		5*time.Second,
		time.Millisecond,
		"reporter should have emitted metric %s", key)
	if !ok {
		c, _ := metricsFactory.Snapshot()
		t.Log("all metrics", c)
	}

	metricsFactory.AssertCounterMetrics(t, []metricstest.ExpectedMetric{
		{Name: "reporter.batches.submitted", Tags: map[string]string{"format": format}, Value: 1},
		{Name: "reporter.spans.submitted", Tags: map[string]string{"format": format}, Value: 1},
		{Name: "thrift.udp.server.packets.processed", Value: 1},
	}...)
}
