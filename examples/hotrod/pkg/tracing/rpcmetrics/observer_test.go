// Copyright (c) 2023 The Jaeger Authors.
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

package rpcmetrics

import (
	"fmt"
	"testing"
	"time"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/stretchr/testify/assert"
	otbridge "go.opentelemetry.io/otel/bridge/opentracing"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"

	u "github.com/jaegertracing/jaeger/internal/metricstest"
)

type testTracer struct {
	metrics *u.Factory
	tracer  opentracing.Tracer
}

func withTestTracer(runTest func(tt *testTracer)) {
	metrics := u.NewFactory(time.Minute)
	observer := NewObserver(metrics, DefaultNameNormalizer)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(observer),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("test"),
		)),
	)
	tracer, _ := otbridge.NewTracerPair(tp.Tracer(""))
	runTest(&testTracer{
		metrics: metrics,
		tracer:  tracer,
	})
}

func TestObserver(t *testing.T) {
	withTestTracer(func(testTracer *testTracer) {
		ts := time.Now()
		finishOptions := opentracing.FinishOptions{
			FinishTime: ts.Add(50 * time.Millisecond),
		}

		testCases := []struct {
			name           string
			tag            opentracing.Tag
			opNameOverride string
			err            bool
		}{
			{name: "local-span", tag: opentracing.Tag{Key: "x", Value: "y"}},
			{name: "get-user", tag: ext.SpanKindRPCServer},
			{name: "get-user", tag: ext.SpanKindRPCServer, opNameOverride: "get-user-override"},
			{name: "get-user", tag: ext.SpanKindRPCServer, err: true},
			{name: "get-user-client", tag: ext.SpanKindRPCClient},
		}

		for _, testCase := range testCases {
			span := testTracer.tracer.StartSpan(
				testCase.name,
				testCase.tag,
				opentracing.StartTime(ts),
			)
			if testCase.opNameOverride != "" {
				span.SetOperationName(testCase.opNameOverride)
			}
			if testCase.err {
				ext.Error.Set(span, true)
			}
			span.FinishWithOptions(finishOptions)
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
		key     string
		variant string
		value   interface{}
		metrics []u.ExpectedMetric
	}

	testCases := []tagTestCase{
		{key: "something", value: 42, metrics: []u.ExpectedMetric{
			{Name: "requests", Value: 1, Tags: tags("error", "false")},
		}},
		{key: "error", value: true, metrics: []u.ExpectedMetric{
			{Name: "requests", Value: 1, Tags: tags("error", "true")},
		}},
		// OTEL bridge does not interpret string "true" as error status
		// {key: "error", value: "true", variant: "string", metrics: []u.ExpectedMetric{
		// 	{Name: "requests", Value: 1, Tags: tags("error", "true")},
		// }},
	}

	for i := 200; i <= 500; i += 100 {
		status_codes := []struct {
			value   interface{}
			variant string
		}{
			{value: i},
			{value: uint16(i), variant: "uint16"},
			{value: fmt.Sprintf("%d", i), variant: "string"},
		}
		for _, v := range status_codes {
			testCases = append(testCases, tagTestCase{
				key:     "http.status_code",
				value:   v.value,
				variant: v.variant,
				metrics: []u.ExpectedMetric{
					{Name: "http_requests", Value: 1, Tags: tags("status_code", fmt.Sprintf("%dxx", i/100))},
				},
			})
		}
	}

	for _, testCase := range testCases {
		for i := range testCase.metrics {
			testCase.metrics[i].Tags["endpoint"] = "span"
		}
		t.Run(fmt.Sprintf("%s-%v-%s", testCase.key, testCase.value, testCase.variant), func(t *testing.T) {
			withTestTracer(func(testTracer *testTracer) {
				span := testTracer.tracer.StartSpan("span", ext.SpanKindRPCServer)
				span.SetTag(testCase.key, testCase.value)
				span.Finish()
				testTracer.metrics.AssertCounterMetrics(t, testCase.metrics...)
			})
		})
	}
}
