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
	jaegerM "github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger-lib/metrics/metricstest"

	"github.com/jaegertracing/jaeger/model"
)

func TestProcessorMetrics(t *testing.T) {
	baseMetrics := metricstest.NewFactory(time.Hour)
	serviceMetrics := baseMetrics.Namespace(jaegerM.NSOptions{Name: "service", Tags: nil})
	hostMetrics := baseMetrics.Namespace(jaegerM.NSOptions{Name: "host", Tags: nil})
	spm := NewSpanProcessorMetrics(serviceMetrics, hostMetrics, []SpanFormat{SpanFormat("scruffy")})
	benderFormatHTTPMetrics := spm.GetCountsForFormat("bender", HTTPTransport)
	assert.NotNil(t, benderFormatHTTPMetrics)
	benderFormatGRPCMetrics := spm.GetCountsForFormat("bender", GRPCTransport)
	assert.NotNil(t, benderFormatGRPCMetrics)

	jTChannelFormat := spm.GetCountsForFormat(JaegerSpanFormat, TChannelTransport)
	assert.NotNil(t, jTChannelFormat)
	jTChannelFormat.ReceivedBySvc.ReportServiceNameForSpan(&model.Span{
		Process: &model.Process{},
	})
	mSpan := model.Span{
		Process: &model.Process{
			ServiceName: "fry",
		},
	}
	jTChannelFormat.ReceivedBySvc.ReportServiceNameForSpan(&mSpan)
	mSpan.Flags.SetDebug()
	jTChannelFormat.ReceivedBySvc.ReportServiceNameForSpan(&mSpan)
	mSpan.ReplaceParentID(1234)
	jTChannelFormat.ReceivedBySvc.ReportServiceNameForSpan(&mSpan)
	counters, gauges := baseMetrics.Backend.Snapshot()

	assert.EqualValues(t, 1, counters["service.spans.received|debug=false|format=jaeger|svc=fry|transport=tchannel"])
	assert.EqualValues(t, 2, counters["service.spans.received|debug=true|format=jaeger|svc=fry|transport=tchannel"])
	assert.EqualValues(t, 1, counters["service.traces.received|debug=false|format=jaeger|sampler_type=unknown|svc=fry|transport=tchannel"])
	assert.EqualValues(t, 1, counters["service.traces.received|debug=true|format=jaeger|sampler_type=unknown|svc=fry|transport=tchannel"])
	assert.Empty(t, gauges)
}

func TestNewTraceCountsBySvc(t *testing.T) {
	baseMetrics := metricstest.NewFactory(time.Hour)
	metrics := newTraceCountsBySvc(baseMetrics, "not_on_my_level", 3)

	metrics.countByServiceName("fry", false, "unknown")
	metrics.countByServiceName("leela", false, "unknown")
	metrics.countByServiceName("bender", false, "unknown")
	metrics.countByServiceName("zoidberg", false, "unknown")

	counters, _ := baseMetrics.Backend.Snapshot()
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=false|sampler_type=unknown|svc=fry"])
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=false|sampler_type=unknown|svc=leela"])
	assert.EqualValues(t, 2, counters["not_on_my_level|debug=false|sampler_type=unknown|svc=other-services"])

	metrics.countByServiceName("zoidberg", true, "remote")
	metrics.countByServiceName("bender", true, "const")
	metrics.countByServiceName("leela", true, "probabilistic")
	metrics.countByServiceName("fry", true, "ratelimiting")
	metrics.countByServiceName("leela", true, "remote")
	metrics.countByServiceName("fry", true, "const")
	metrics.countByServiceName("elzar", true, "lowerbound")
	metrics.countByServiceName("url", true, "unknown")

	counters, _ = baseMetrics.Backend.Snapshot()
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=true|sampler_type=remote|svc=zoidberg"])
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=true|sampler_type=const|svc=bender"])
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=true|sampler_type=probabilistic|svc=other-services"])
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=true|sampler_type=ratelimiting|svc=other-services"])
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=true|sampler_type=remote|svc=other-services"])
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=true|sampler_type=const|svc=other-services"])
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=true|sampler_type=lowerbound|svc=other-services"])
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=true|sampler_type=unknown|svc=other-services"])
}

func TestNewSpanCountsBySvc(t *testing.T) {
	baseMetrics := metricstest.NewFactory(time.Hour)
	metrics := newSpanCountsBySvc(baseMetrics, "not_on_my_level", 3)
	const spanSamplerType = ""
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
	key := tc.buildKey("sample-service", "unknown")
	assert.Equal(t, "sample-service$_$unknown", key)
	key = tc.buildKey("sample-service2", "const")
	assert.Equal(t, "sample-service2$_$const", key)
}
