// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package reporter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger-idl/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger-idl/thrift-gen/zipkincore"
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
			err := reporter.EmitBatch(context.Background(), nil)
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
			err := reporter.EmitBatch(context.Background(), &jaeger.Batch{Spans: []*jaeger.Span{{}}})
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
			err := reporter.EmitZipkinBatch(context.Background(), nil)
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
			err := reporter.EmitZipkinBatch(context.Background(), []*zipkincore.Span{{}})
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
			err := reporter.EmitBatch(context.Background(), &jaeger.Batch{Spans: []*jaeger.Span{{}}})
			require.Error(t, err)
		}, rep: &mockReporter{err: errors.New("foo")}},
		{expectedCounters: []metricstest.ExpectedMetric{
			{Name: "reporter.batches.failures", Tags: map[string]string{"format": "zipkin"}, Value: 1},
			{Name: "reporter.spans.failures", Tags: map[string]string{"format": "zipkin"}, Value: 2},
		}, expectedGauges: []metricstest.ExpectedMetric{
			{Name: "reporter.batch_size", Tags: map[string]string{"format": "zipkin"}, Value: 0},
		}, action: func(reporter Reporter) {
			err := reporter.EmitZipkinBatch(context.Background(), []*zipkincore.Span{{}, {}})
			require.Error(t, err)
		}, rep: &mockReporter{errors.New("foo")}},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			metricsFactory := metricstest.NewFactory(time.Microsecond)
			defer metricsFactory.Stop()
			r := WrapWithMetrics(test.rep, metricsFactory)
			test.action(r)
			metricsFactory.AssertCounterMetrics(t, test.expectedCounters...)
			metricsFactory.AssertGaugeMetrics(t, test.expectedGauges...)
		})
	}
}
