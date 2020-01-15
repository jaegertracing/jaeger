// Copyright (c) 2018 The Jaeger Authors.
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

package reporter

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics/metricstest"

	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

func TestMetricsReporter(t *testing.T) {
	tests := []struct {
		expectedCounters []metricstest.ExpectedMetric
		expectedGauges   []metricstest.ExpectedMetric
		action           func(reporter Reporter)
		rep              *mockReporter
	}{
		{expectedCounters: []metricstest.ExpectedMetric{
			{Name: "reporter.batches.submitted", Tags: map[string]string{"format": "jaeger"}, Value: 1},
			{Name: "reporter.batches.failures", Tags: map[string]string{"format": "jaeger"}, Value: 0},
			{Name: "reporter.spans.submitted", Tags: map[string]string{"format": "jaeger"}, Value: 0},
			{Name: "reporter.spans.failures", Tags: map[string]string{"format": "jaeger"}, Value: 0},
		}, expectedGauges: []metricstest.ExpectedMetric{
			{Name: "reporter.batch_size", Tags: map[string]string{"format": "jaeger"}, Value: 0},
		}, action: func(reporter Reporter) {
			err := reporter.EmitBatch(nil)
			require.NoError(t, err)
		}, rep: &mockReporter{}},
		{expectedCounters: []metricstest.ExpectedMetric{
			{Name: "reporter.batches.submitted", Tags: map[string]string{"format": "jaeger"}, Value: 1},
			{Name: "reporter.batches.failures", Tags: map[string]string{"format": "jaeger"}, Value: 0},
			{Name: "reporter.spans.submitted", Tags: map[string]string{"format": "jaeger"}, Value: 1},
			{Name: "reporter.spans.failures", Tags: map[string]string{"format": "jaeger"}, Value: 0},
		}, expectedGauges: []metricstest.ExpectedMetric{
			{Name: "reporter.batch_size", Tags: map[string]string{"format": "jaeger"}, Value: 1},
		}, action: func(reporter Reporter) {
			err := reporter.EmitBatch(&jaeger.Batch{Spans: []*jaeger.Span{{}}})
			require.NoError(t, err)
		}, rep: &mockReporter{}},
		{expectedCounters: []metricstest.ExpectedMetric{
			{Name: "reporter.batches.submitted", Tags: map[string]string{"format": "zipkin"}, Value: 1},
			{Name: "reporter.batches.failures", Tags: map[string]string{"format": "zipkin"}, Value: 0},
			{Name: "reporter.spans.submitted", Tags: map[string]string{"format": "zipkin"}, Value: 0},
			{Name: "reporter.spans.failures", Tags: map[string]string{"format": "zipkin"}, Value: 0},
		}, expectedGauges: []metricstest.ExpectedMetric{
			{Name: "reporter.batch_size", Tags: map[string]string{"format": "zipkin"}, Value: 0},
		}, action: func(reporter Reporter) {
			err := reporter.EmitZipkinBatch(nil)
			require.NoError(t, err)
		}, rep: &mockReporter{}},
		{expectedCounters: []metricstest.ExpectedMetric{
			{Name: "reporter.batches.submitted", Tags: map[string]string{"format": "zipkin"}, Value: 1},
			{Name: "reporter.batches.failures", Tags: map[string]string{"format": "zipkin"}, Value: 0},
			{Name: "reporter.spans.submitted", Tags: map[string]string{"format": "zipkin"}, Value: 1},
			{Name: "reporter.spans.failures", Tags: map[string]string{"format": "zipkin"}, Value: 0},
		}, expectedGauges: []metricstest.ExpectedMetric{
			{Name: "reporter.batch_size", Tags: map[string]string{"format": "zipkin"}, Value: 1},
		}, action: func(reporter Reporter) {
			err := reporter.EmitZipkinBatch([]*zipkincore.Span{{}})
			require.NoError(t, err)
		}, rep: &mockReporter{}},
		{expectedCounters: []metricstest.ExpectedMetric{
			{Name: "reporter.batches.submitted", Tags: map[string]string{"format": "jaeger"}, Value: 0},
			{Name: "reporter.batches.failures", Tags: map[string]string{"format": "jaeger"}, Value: 1},
			{Name: "reporter.spans.submitted", Tags: map[string]string{"format": "jaeger"}, Value: 0},
			{Name: "reporter.spans.failures", Tags: map[string]string{"format": "jaeger"}, Value: 1},
		}, expectedGauges: []metricstest.ExpectedMetric{
			{Name: "reporter.batch_size", Tags: map[string]string{"format": "jaeger"}, Value: 0},
		}, action: func(reporter Reporter) {
			err := reporter.EmitBatch(&jaeger.Batch{Spans: []*jaeger.Span{{}}})
			require.Error(t, err)
		}, rep: &mockReporter{err: errors.New("foo")}},
		{expectedCounters: []metricstest.ExpectedMetric{
			{Name: "reporter.batches.failures", Tags: map[string]string{"format": "zipkin"}, Value: 1},
			{Name: "reporter.spans.failures", Tags: map[string]string{"format": "zipkin"}, Value: 2},
		}, expectedGauges: []metricstest.ExpectedMetric{
			{Name: "reporter.batch_size", Tags: map[string]string{"format": "zipkin"}, Value: 0},
		}, action: func(reporter Reporter) {
			err := reporter.EmitZipkinBatch([]*zipkincore.Span{{}, {}})
			require.Error(t, err)
		}, rep: &mockReporter{errors.New("foo")}},
	}

	for _, test := range tests {
		metricsFactory := metricstest.NewFactory(time.Microsecond)
		r := WrapWithMetrics(test.rep, metricsFactory)
		test.action(r)
		metricsFactory.AssertCounterMetrics(t, test.expectedCounters...)
		metricsFactory.AssertGaugeMetrics(t, test.expectedGauges...)
	}
}
