// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/metricstest"
)

func TestProcessorMetrics(t *testing.T) {
	baseMetrics := metricstest.NewFactory(time.Hour)
	defer baseMetrics.Backend.Stop()
	serviceMetrics := baseMetrics.Namespace(metrics.NSOptions{Name: "service", Tags: nil})
	hostMetrics := baseMetrics.Namespace(metrics.NSOptions{Name: "host", Tags: nil})
	spm := NewSpanProcessorMetrics(serviceMetrics, hostMetrics, []processor.SpanFormat{processor.SpanFormat("scruffy")})
	benderFormatHTTPMetrics := spm.GetCountsForFormat("bender", processor.HTTPTransport)
	assert.NotNil(t, benderFormatHTTPMetrics)
	benderFormatGRPCMetrics := spm.GetCountsForFormat("bender", processor.GRPCTransport)
	assert.NotNil(t, benderFormatGRPCMetrics)

	grpcChannelFormat := spm.GetCountsForFormat(processor.JaegerSpanFormat, processor.GRPCTransport)
	assert.NotNil(t, grpcChannelFormat)
	grpcChannelFormat.ReceivedBySvc.ForSpanV1(&model.Span{
		Process: &model.Process{},
	})
	mSpan := model.Span{
		Process: &model.Process{
			ServiceName: "fry",
		},
	}
	grpcChannelFormat.ReceivedBySvc.ForSpanV1(&mSpan)
	mSpan.Flags.SetDebug()
	grpcChannelFormat.ReceivedBySvc.ForSpanV1(&mSpan)
	mSpan.ReplaceParentID(1234)
	grpcChannelFormat.ReceivedBySvc.ForSpanV1(&mSpan)

	pd := ptrace.NewTraces()
	rs := pd.ResourceSpans().AppendEmpty()
	resource := rs.Resource()
	resource.Attributes().PutStr("service.name", "fry")
	sp := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	grpcChannelFormat.ReceivedBySvc.ForSpanV2(resource, sp)
	sp.SetParentSpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8})
	grpcChannelFormat.ReceivedBySvc.ForSpanV2(resource, sp)

	counters, gauges := baseMetrics.Backend.Snapshot()

	assert.EqualValues(t, 3, counters["service.spans.received|debug=false|format=jaeger|svc=fry|transport=grpc"])
	assert.EqualValues(t, 2, counters["service.spans.received|debug=true|format=jaeger|svc=fry|transport=grpc"])
	assert.EqualValues(t, 2, counters["service.traces.received|debug=false|format=jaeger|sampler_type=unrecognized|svc=fry|transport=grpc"])
	assert.EqualValues(t, 1, counters["service.traces.received|debug=true|format=jaeger|sampler_type=unrecognized|svc=fry|transport=grpc"])
	assert.Empty(t, gauges)
}

func TestNewTraceCountsBySvc(t *testing.T) {
	baseMetrics := metricstest.NewFactory(time.Hour)
	defer baseMetrics.Backend.Stop()
	svcMetrics := newTraceCountsBySvc(baseMetrics, "not_on_my_level", 3)

	svcMetrics.countByServiceName("fry", false, model.SamplerTypeUnrecognized)
	svcMetrics.countByServiceName("leela", false, model.SamplerTypeUnrecognized)
	svcMetrics.countByServiceName("bender", false, model.SamplerTypeUnrecognized)
	svcMetrics.countByServiceName("zoidberg", false, model.SamplerTypeUnrecognized)

	counters, _ := baseMetrics.Backend.Snapshot()
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=false|sampler_type=unrecognized|svc=fry"])
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=false|sampler_type=unrecognized|svc=leela"])
	assert.EqualValues(t, 2, counters["not_on_my_level|debug=false|sampler_type=unrecognized|svc=other-services"], counters)

	svcMetrics.countByServiceName("bender", true, model.SamplerTypeConst)
	svcMetrics.countByServiceName("bender", true, model.SamplerTypeProbabilistic)
	svcMetrics.countByServiceName("leela", true, model.SamplerTypeProbabilistic)
	svcMetrics.countByServiceName("fry", true, model.SamplerTypeRateLimiting)
	svcMetrics.countByServiceName("fry", true, model.SamplerTypeConst)
	svcMetrics.countByServiceName("elzar", true, model.SamplerTypeLowerBound)
	svcMetrics.countByServiceName("url", true, model.SamplerTypeUnrecognized)

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
	defer baseMetrics.Backend.Stop()
	svcMetrics := newSpanCountsBySvc(baseMetrics, "not_on_my_level", 3)
	svcMetrics.countByServiceName("fry", false)
	svcMetrics.countByServiceName("leela", false)
	svcMetrics.countByServiceName("bender", false)
	svcMetrics.countByServiceName("zoidberg", false)

	counters, _ := baseMetrics.Backend.Snapshot()
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=false|svc=fry"])
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=false|svc=leela"])
	assert.EqualValues(t, 2, counters["not_on_my_level|debug=false|svc=other-services"])

	svcMetrics.countByServiceName("zoidberg", true)
	svcMetrics.countByServiceName("bender", true)
	svcMetrics.countByServiceName("leela", true)
	svcMetrics.countByServiceName("fry", true)

	counters, _ = baseMetrics.Backend.Snapshot()
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=true|svc=zoidberg"])
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=true|svc=bender"])
	assert.EqualValues(t, 2, counters["not_on_my_level|debug=true|svc=other-services"])
}

func TestBuildKey(t *testing.T) {
	// This test checks if stringBuilder is reset every time buildKey is called.
	tc := newTraceCountsBySvc(metrics.NullFactory, "received", 100)
	key := tc.buildKey("sample-service", model.SamplerTypeUnrecognized.String())
	assert.Equal(t, "sample-service$_$unrecognized", key)
	key = tc.buildKey("sample-service2", model.SamplerTypeConst.String())
	assert.Equal(t, "sample-service2$_$const", key)
}
