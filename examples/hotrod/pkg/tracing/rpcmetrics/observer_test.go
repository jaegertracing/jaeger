// Copyright (c) 2023 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package rpcmetrics

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	u "github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/pkg/otelsemconv"
)

type testTracer struct {
	metrics *u.Factory
	tracer  trace.Tracer
}

func withTestTracer(runTest func(tt *testTracer)) {
	metrics := u.NewFactory(time.Minute)
	defer metrics.Stop()
	observer := NewObserver(metrics, DefaultNameNormalizer)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(observer),
		sdktrace.WithResource(resource.NewWithAttributes(
			otelsemconv.SchemaURL,
			otelsemconv.ServiceNameKey.String("test"),
		)),
	)
	runTest(&testTracer{
		metrics: metrics,
		tracer:  tp.Tracer("test"),
	})
}

func TestObserver(t *testing.T) {
	withTestTracer(func(testTracer *testTracer) {
		ts := time.Now()
		finishOptions := trace.WithTimestamp(ts.Add((50 * time.Millisecond)))

		testCases := []struct {
			name           string
			spanKind       trace.SpanKind
			opNameOverride string
			err            bool
		}{
			{name: "local-span", spanKind: trace.SpanKindInternal},
			{name: "get-user", spanKind: trace.SpanKindServer},
			{name: "get-user", spanKind: trace.SpanKindServer, opNameOverride: "get-user-override"},
			{name: "get-user", spanKind: trace.SpanKindServer, err: true},
			{name: "get-user-client", spanKind: trace.SpanKindClient},
		}

		for _, testCase := range testCases {
			_, span := testTracer.tracer.Start(
				context.Background(),
				testCase.name, trace.WithSpanKind(testCase.spanKind),
				trace.WithTimestamp(ts),
			)
			if testCase.opNameOverride != "" {
				span.SetName(testCase.opNameOverride)
			}
			if testCase.err {
				span.SetStatus(codes.Error, "An error occurred")
			}
			span.End(finishOptions)
		}

		testTracer.metrics.AssertCounterMetrics(t,
			u.ExpectedMetric{Name: "requests", Tags: endpointTags("local_span", "error", "false"), Value: 0},
			u.ExpectedMetric{Name: "requests", Tags: endpointTags("get_user", "error", "false"), Value: 1},
			u.ExpectedMetric{Name: "requests", Tags: endpointTags("get_user", "error", "true"), Value: 1},
			u.ExpectedMetric{Name: "requests", Tags: endpointTags("get_user_override", "error", "false"), Value: 1},
			u.ExpectedMetric{Name: "requests", Tags: endpointTags("get_user_client", "error", "false"), Value: 0},
		)
		// TODO something wrong with string generation, .P99 should not be appended to the tag
		// as a result we cannot use u.AssertGaugeMetrics
		_, g := testTracer.metrics.Snapshot()
		assert.EqualValues(t, 51, g["request_latency|endpoint=get_user|error=false.P99"])
		assert.EqualValues(t, 51, g["request_latency|endpoint=get_user|error=true.P99"])
	})
}

func TestTags(t *testing.T) {
	type tagTestCase struct {
		attr    attribute.KeyValue
		err     bool
		metrics []u.ExpectedMetric
	}
	testCases := []tagTestCase{
		{err: false, metrics: []u.ExpectedMetric{
			{Name: "requests", Value: 1, Tags: tags("error", "false")},
		}},
		{err: true, metrics: []u.ExpectedMetric{
			{Name: "requests", Value: 1, Tags: tags("error", "true")},
		}},
	}

	for i := 200; i <= 500; i += 100 {
		testCases = append(testCases, tagTestCase{
			attr: otelsemconv.HTTPResponseStatusCode(i),
			metrics: []u.ExpectedMetric{
				{Name: "http_requests", Value: 1, Tags: tags("status_code", fmt.Sprintf("%dxx", i/100))},
			},
		})
	}

	for _, testCase := range testCases {
		for i := range testCase.metrics {
			testCase.metrics[i].Tags["endpoint"] = "span"
		}
		t.Run(fmt.Sprintf("%s-%v", testCase.attr.Key, testCase.attr.Value), func(t *testing.T) {
			withTestTracer(func(testTracer *testTracer) {
				_, span := testTracer.tracer.Start(
					context.Background(),
					"span", trace.WithSpanKind(trace.SpanKindServer),
				)
				span.SetAttributes(testCase.attr)
				if testCase.err {
					span.SetStatus(codes.Error, "An error occurred")
				}
				span.End()
				testTracer.metrics.AssertCounterMetrics(t, testCase.metrics...)
			})
		})
	}
}
