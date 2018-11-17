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

package tchannel

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/agent/app/testutils"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

func initRequirements(t *testing.T) (*metrics.LocalFactory, *testutils.MockTCollector, *Reporter) {
	metricsFactory, collector := testutils.InitMockCollector(t)
	reporter := New("jaeger-collector", collector.Channel, time.Second, nil, zap.NewNop())
	return metricsFactory, collector, reporter
}

func TestZipkinTChannelReporterSuccess(t *testing.T) {
	_, collector, reporter := initRequirements(t)
	defer collector.Close()

	require.NoError(t, submitTestZipkinBatch(reporter))

	// TODO potentially flaky test
	time.Sleep(100 * time.Millisecond) // wait for server to receive

	require.Equal(t, 1, len(collector.GetZipkinSpans()))
	assert.Equal(t, "span1", collector.GetZipkinSpans()[0].Name)
}

func TestZipkinTChannelReporterFailure(t *testing.T) {
	_, collector, reporter := initRequirements(t)
	defer collector.Close()

	collector.ReturnErr = true

	require.Error(t, submitTestZipkinBatch(reporter))
}

func submitTestZipkinBatch(reporter *Reporter) error {
	span := zipkincore.NewSpan()
	span.Name = "span1"

	return reporter.EmitZipkinBatch([]*zipkincore.Span{span})
}

func TestJaegerTChannelReporterSuccess(t *testing.T) {
	_, collector, reporter := initRequirements(t)
	defer collector.Close()

	require.NoError(t, submitTestJaegerBatch(reporter))

	time.Sleep(100 * time.Millisecond) // wait for server to receive

	require.Equal(t, 1, len(collector.GetJaegerBatches()))
	assert.Equal(t, "span1", collector.GetJaegerBatches()[0].Spans[0].OperationName)
}

func TestJaegerTChannelReporterFailure(t *testing.T) {
	_, collector, reporter := initRequirements(t)
	defer collector.Close()

	collector.ReturnErr = true

	require.Error(t, submitTestJaegerBatch(reporter))
}

func submitTestJaegerBatch(reporter *Reporter) error {
	batch := jaeger.NewBatch()
	batch.Process = jaeger.NewProcess()
	batch.Spans = []*jaeger.Span{{OperationName: "span1"}}

	return reporter.EmitBatch(batch)
}
