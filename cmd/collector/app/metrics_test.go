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
	spm := NewSpanProcessorMetrics(serviceMetrics, hostMetrics, []string{"scruffy"})
	benderFormatMetrics := spm.GetCountsForFormat("bender")
	assert.NotNil(t, benderFormatMetrics)
	jFormat := spm.GetCountsForFormat(JaegerFormatType)
	assert.NotNil(t, jFormat)
	jFormat.ReceivedBySvc.ReportServiceNameForSpan(&model.Span{
		Process: &model.Process{},
	}, TChannelEndpoint)
	mSpan := model.Span{
		Process: &model.Process{
			ServiceName: "fry",
		},
	}
	jFormat.ReceivedBySvc.ReportServiceNameForSpan(&mSpan, TChannelEndpoint)
	mSpan.Flags.SetDebug()
	jFormat.ReceivedBySvc.ReportServiceNameForSpan(&mSpan, TChannelEndpoint)
	mSpan.ReplaceParentID(1234)
	jFormat.ReceivedBySvc.ReportServiceNameForSpan(&mSpan, TChannelEndpoint)
	counters, gauges := baseMetrics.Backend.Snapshot()

	assert.EqualValues(t, 1, counters["service.spans.received|debug=false|format=jaeger|svc=fry|transport="+TChannelEndpoint])
	assert.EqualValues(t, 2, counters["service.spans.received|debug=true|format=jaeger|svc=fry|transport="+TChannelEndpoint])
	assert.EqualValues(t, 1, counters["service.traces.received|debug=false|format=jaeger|svc=fry|transport="+TChannelEndpoint])
	assert.EqualValues(t, 1, counters["service.traces.received|debug=true|format=jaeger|svc=fry|transport="+TChannelEndpoint])
	assert.Empty(t, gauges)
}

func TestNewCountsBySvc(t *testing.T) {
	baseMetrics := metricstest.NewFactory(time.Hour)
	metrics := newCountsBySvc(baseMetrics, "not_on_my_level", 3)

	metrics.countByServiceName("fry", false, "")
	metrics.countByServiceName("leela", false, "")
	metrics.countByServiceName("bender", false, "")
	metrics.countByServiceName("zoidberg", false, "")
	metrics.countByServiceName("zoidberg2", false, TChannelEndpoint)
	metrics.countByServiceName("zoidberg3", false, HTTPEndpoint)

	counters, _ := baseMetrics.Backend.Snapshot()
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=false|svc=fry|transport="+defaultTransportType])
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=false|svc=leela|transport="+defaultTransportType])
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=false|svc=other-services|transport="+TChannelEndpoint])
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=false|svc=other-services|transport="+HTTPEndpoint])

	metrics.countByServiceName("zoidberg", true, GRPCEndpoint)
	metrics.countByServiceName("bender", true, GRPCEndpoint)
	metrics.countByServiceName("leela", true, GRPCEndpoint)
	metrics.countByServiceName("fry", true, GRPCEndpoint)

	counters, _ = baseMetrics.Backend.Snapshot()
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=true|svc=zoidberg|transport="+GRPCEndpoint])
	assert.EqualValues(t, 1, counters["not_on_my_level|debug=true|svc=bender|transport="+GRPCEndpoint])
	assert.EqualValues(t, 2, counters["not_on_my_level|debug=true|svc=other-services|transport="+GRPCEndpoint])
}
