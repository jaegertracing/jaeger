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

package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/model"
	jaegerM "github.com/jaegertracing/jaeger/pkg/metrics"
)

func TestProcessorMetrics(t *testing.T) {
	baseMetrics := metricstest.NewFactory(time.Hour)
	serviceMetrics := baseMetrics.Namespace(jaegerM.NSOptions{Name: "service", Tags: nil})
	hostMetrics := baseMetrics.Namespace(jaegerM.NSOptions{Name: "host", Tags: nil})
	spm := NewSpanProcessorMetrics(serviceMetrics, hostMetrics, []processor.SpanFormat{processor.SpanFormat("scruffy")})
	benderFormatHTTPMetrics := spm.GetCountsForFormat("bender", processor.HTTPTransport)
	assert.NotNil(t, benderFormatHTTPMetrics)
	benderFormatGRPCMetrics := spm.GetCountsForFormat("bender", processor.GRPCTransport)
	assert.NotNil(t, benderFormatGRPCMetrics)

	grpcChannelFormat := spm.GetCountsForFormat(processor.JaegerSpanFormat, processor.GRPCTransport)
	assert.NotNil(t, grpcChannelFormat)
	grpcChannelFormat.ReceivedBySvc.ReportServiceNameForSpan(&model.Span{
		Process: &model.Process{},
	})
	mSpan := model.Span{
		Process: &model.Process{
			ServiceName: "fry",
		},
	}
	grpcChannelFormat.ReceivedBySvc.ReportServiceNameForSpan(&mSpan)
	mSpan.Flags.SetDebug()
	grpcChannelFormat.ReceivedBySvc.ReportServiceNameForSpan(&mSpan)
	mSpan.ReplaceParentID(1234)
	grpcChannelFormat.ReceivedBySvc.ReportServiceNameForSpan(&mSpan)
	counters, gauges := baseMetrics.Backend.Snapshot()

	assert.EqualValues(t, 1, counters["service.spans.received|debug=false|format=jaeger|svc=fry|transport=grpc"])
	assert.EqualValues(t, 2, counters["service.spans.received|debug=true|format=jaeger|svc=fry|transport=grpc"])
	assert.EqualValues(t, 1, counters["service.traces.received|debug=false|format=jaeger|sampler_type=unrecognized|svc=fry|transport=grpc"])
	assert.EqualValues(t, 1, counters["service.traces.received|debug=true|format=jaeger|sampler_type=unrecognized|svc=fry|transport=grpc"])
	assert.Empty(t, gauges)
}

func TestNewTraceCountsBySvc(t *testing.T) {
	baseMetrics := metricstest.NewFactory(time.Hour)
	metrics := newTraceCountsBySvc(baseMetrics, "not_on_my_level", 3)

	metrics.countByServiceName("fry", false, model.SamplerTypeUnrecognized)
	metrics.countByServiceName("leela", false, model.SamplerTypeUnrecognized)
	metrics.countByServiceName("bender", false, model.SamplerTypeUnrecognized)
	metrics.countByServiceName("zoidberg", false, model.SamplerTypeUnrecognized)

	counters, _ := baseMetrics.Backend.Snapshot()
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=false|sampler_type=unrecognized|svc=fry"])
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=false|sampler_type=unrecognized|svc=leela"])
	assert.EqualValues(t, 2, counters["not_on_my_level|debug=false|sampler_type=unrecognized|svc=other-services"], counters)

	metrics.countByServiceName("bender", true, model.SamplerTypeConst)
	metrics.countByServiceName("bender", true, model.SamplerTypeProbabilistic)
	metrics.countByServiceName("leela", true, model.SamplerTypeProbabilistic)
	metrics.countByServiceName("fry", true, model.SamplerTypeRateLimiting)
	metrics.countByServiceName("fry", true, model.SamplerTypeConst)
	metrics.countByServiceName("elzar", true, model.SamplerTypeLowerBound)
	metrics.countByServiceName("url", true, model.SamplerTypeUnrecognized)

	counters, _ = baseMetrics.Backend.Snapshot()
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=true|sampler_type=const|svc=bender"])
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=true|sampler_type=probabilistic|svc=bender"])
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=true|sampler_type=probabilistic|svc=other-services"], counters)
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=true|sampler_type=ratelimiting|svc=other-services"])
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=true|sampler_type=const|svc=other-services"])
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=true|sampler_type=lowerbound|svc=other-services"])
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=true|sampler_type=unrecognized|svc=other-services"])
}

func TestNewSpanCountsBySvc(t *testing.T) {
	baseMetrics := metricstest.NewFactory(time.Hour)
	metrics := newSpanCountsBySvc(baseMetrics, "not_on_my_level", 3)
	metrics.countByServiceName("fry", false)
	metrics.countByServiceName("leela", false)
	metrics.countByServiceName("bender", false)
	metrics.countByServiceName("zoidberg", false)

	counters, _ := baseMetrics.Backend.Snapshot()
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=false|svc=fry"])
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=false|svc=leela"])
	assert.EqualValues(t, 2, counters["not_on_my_level|debug=false|svc=other-services"])

	metrics.countByServiceName("zoidberg", true)
	metrics.countByServiceName("bender", true)
	metrics.countByServiceName("leela", true)
	metrics.countByServiceName("fry", true)

	counters, _ = baseMetrics.Backend.Snapshot()
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=true|svc=zoidberg"])
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=true|svc=bender"])
	assert.EqualValues(t, 2, counters["not_on_my_level|debug=true|svc=other-services"])
}

func TestBuildKey(t *testing.T) {
	// This test checks if stringBuilder is reset every time buildKey is called.
	tc := newTraceCountsBySvc(jaegerM.NullFactory, "received", 100)
	key := tc.buildKey("sample-service", model.SamplerTypeUnrecognized.String())
	assert.Equal(t, "sample-service$_$unrecognized", key)
	key = tc.buildKey("sample-service2", model.SamplerTypeConst.String())
	assert.Equal(t, "sample-service2$_$const", key)
}
