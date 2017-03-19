package reporter

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/zap"
	"github.com/uber/jaeger-lib/metrics"
	mTestutils "github.com/uber/jaeger-lib/metrics/testutils"

	"github.com/uber/jaeger/thrift-gen/jaeger"
	"github.com/uber/jaeger/thrift-gen/zipkincore"

	"code.uber.internal/infra/jaeger/oss/cmd/agent/app/testutils"
)

func initRequirements(t *testing.T) (*metrics.LocalFactory, *testutils.MockTCollector, Reporter) {
	metricsFactory, collector := testutils.InitMockCollector(t)

	reporter := NewTCollectorReporter(collector.Channel, metricsFactory, zap.New(zap.NullEncoder()), nil)

	return metricsFactory, collector, reporter
}

func TestZipkinTCollectorReporterSuccess(t *testing.T) {
	metricsFactory, collector, reporter := initRequirements(t)
	defer collector.Close()

	require.NoError(t, submitTestZipkinBatch(reporter))

	time.Sleep(100 * time.Millisecond) // wait for server to receive

	require.Equal(t, 1, len(collector.GetZipkinSpans()))
	assert.Equal(t, "span1", collector.GetZipkinSpans()[0].Name)

	// agentReporter must emit metrics
	checkCounters(t, metricsFactory, 1, 1, 0, 0, "zipkin")
}

func TestZipkinTCollectorReporterFailure(t *testing.T) {
	metricsFactory, collector, reporter := initRequirements(t)
	defer collector.Close()

	collector.ReturnErr = true

	require.Error(t, submitTestZipkinBatch(reporter))

	checkCounters(t, metricsFactory, 0, 0, 1, 1, "zipkin")
}

func submitTestZipkinBatch(reporter Reporter) error {
	span := zipkincore.NewSpan()
	span.Name = "span1"

	return reporter.EmitZipkinBatch([]*zipkincore.Span{span})
}

func TestJaegerTCollectorReporterSuccess(t *testing.T) {
	metricsFactory, collector, reporter := initRequirements(t)
	defer collector.Close()

	require.NoError(t, submitTestJaegerBatch(reporter))

	time.Sleep(100 * time.Millisecond) // wait for server to receive

	require.Equal(t, 1, len(collector.GetJaegerBatches()))
	assert.Equal(t, "span1", collector.GetJaegerBatches()[0].Spans[0].OperationName)

	// agentReporter must emit metrics
	checkCounters(t, metricsFactory, 1, 1, 0, 0, "jaeger")
}

func TestJaegerTCollectorReporterFailure(t *testing.T) {
	metricsFactory, collector, reporter := initRequirements(t)
	defer collector.Close()

	collector.ReturnErr = true

	require.Error(t, submitTestJaegerBatch(reporter))

	// agentReporter must emit metrics
	checkCounters(t, metricsFactory, 0, 0, 1, 1, "jaeger")
}

func submitTestJaegerBatch(reporter Reporter) error {
	batch := jaeger.NewBatch()
	batch.Process = jaeger.NewProcess()
	batch.Spans = []*jaeger.Span{{OperationName: "span1"}}

	return reporter.EmitBatch(batch)
}

func checkCounters(t *testing.T, mf *metrics.LocalFactory, batchesSubmitted, spansSubmitted, batchesFailures, spansFailures int, prefix string) {
	batchesCounter := fmt.Sprintf("tc-reporter.%s.batches.submitted", prefix)
	batchesFailureCounter := fmt.Sprintf("tc-reporter.%s.batches.failures", prefix)
	spansCounter := fmt.Sprintf("tc-reporter.%s.spans.submitted", prefix)
	spansFailureCounter := fmt.Sprintf("tc-reporter.%s.spans.failures", prefix)

	mTestutils.AssertCounterMetrics(t, mf, []mTestutils.ExpectedMetric{
		{Name: batchesCounter, Value: batchesSubmitted},
		{Name: spansCounter, Value: spansSubmitted},
		{Name: batchesFailureCounter, Value: batchesFailures},
		{Name: spansFailureCounter, Value: spansFailures},
	}...)
}
