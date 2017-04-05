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

package reporter

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	mTestutils "github.com/uber/jaeger-lib/metrics/testutils"
	"go.uber.org/zap"

	"github.com/uber/jaeger/cmd/agent/app/testutils"
	"github.com/uber/jaeger/thrift-gen/jaeger"
	"github.com/uber/jaeger/thrift-gen/zipkincore"
)

func initRequirements(t *testing.T) (*metrics.LocalFactory, *testutils.MockTCollector, Reporter) {
	metricsFactory, collector := testutils.InitMockCollector(t)
	reporter := NewTChannelReporter("jaeger-collector", collector.Channel, metricsFactory, zap.NewNop(), nil)
	return metricsFactory, collector, reporter
}

func TestZipkinTChannelReporterSuccess(t *testing.T) {
	metricsFactory, collector, reporter := initRequirements(t)
	defer collector.Close()

	require.NoError(t, submitTestZipkinBatch(reporter))

	time.Sleep(100 * time.Millisecond) // wait for server to receive

	require.Equal(t, 1, len(collector.GetZipkinSpans()))
	assert.Equal(t, "span1", collector.GetZipkinSpans()[0].Name)

	// agentReporter must emit metrics
	checkCounters(t, metricsFactory, 1, 1, 0, 0, "zipkin")
}

func TestZipkinTChannelReporterFailure(t *testing.T) {
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

func TestJaegerTChannelReporterSuccess(t *testing.T) {
	metricsFactory, collector, reporter := initRequirements(t)
	defer collector.Close()

	require.NoError(t, submitTestJaegerBatch(reporter))

	time.Sleep(100 * time.Millisecond) // wait for server to receive

	require.Equal(t, 1, len(collector.GetJaegerBatches()))
	assert.Equal(t, "span1", collector.GetJaegerBatches()[0].Spans[0].OperationName)

	// agentReporter must emit metrics
	checkCounters(t, metricsFactory, 1, 1, 0, 0, "jaeger")
}

func TestJaegerTChannelReporterFailure(t *testing.T) {
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
