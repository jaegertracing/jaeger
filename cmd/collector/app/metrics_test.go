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

	"github.com/jaegertracing/jaeger/model"
)

func TestProcessorMetrics(t *testing.T) {
	baseMetrics := jaegerM.NewLocalFactory(time.Hour)
	serviceMetrics := baseMetrics.Namespace("service", nil)
	hostMetrics := baseMetrics.Namespace("host", nil)
	spm := NewSpanProcessorMetrics(serviceMetrics, hostMetrics, []string{"scruffy"})
	benderFormatMetrics := spm.GetCountsForFormat("bender")
	assert.NotNil(t, benderFormatMetrics)
	jFormat := spm.GetCountsForFormat(JaegerFormatType)
	assert.NotNil(t, jFormat)
	jFormat.ReceivedBySvc.ReportServiceNameForSpan(&model.Span{
		Process: &model.Process{},
	})
	mSpan := model.Span{
		Process: &model.Process{
			ServiceName: "fry",
		},
	}
	jFormat.ReceivedBySvc.ReportServiceNameForSpan(&mSpan)
	mSpan.Flags.SetDebug()
	jFormat.ReceivedBySvc.ReportServiceNameForSpan(&mSpan)
	mSpan.ReplaceParentID(1234)
	jFormat.ReceivedBySvc.ReportServiceNameForSpan(&mSpan)
	counters, gauges := baseMetrics.LocalBackend.Snapshot()

	assert.EqualValues(t, 1, counters["service.spans.received|debug=false|format=jaeger|service=fry"])
	assert.EqualValues(t, 2, counters["service.spans.received|debug=true|format=jaeger|service=fry"])
	assert.EqualValues(t, 1, counters["service.traces.received|debug=false|format=jaeger|service=fry"])
	assert.EqualValues(t, 1, counters["service.traces.received|debug=true|format=jaeger|service=fry"])
	assert.Empty(t, gauges)
}

func TestNewCountsBySvc(t *testing.T) {
	baseMetrics := jaegerM.NewLocalFactory(time.Hour)
	metrics := newCountsBySvc(baseMetrics, 3)

	metrics.countByServiceName("fry", false)
	metrics.countByServiceName("leela", false)
	metrics.countByServiceName("bender", false)
	metrics.countByServiceName("zoidberg", false)

	counters, _ := baseMetrics.LocalBackend.Snapshot()
	assert.EqualValues(t, 1, counters["fry|debug=false"])
	assert.EqualValues(t, 1, counters["leela|debug=false"])
	assert.EqualValues(t, 2, counters["other-services|debug=false"])

	metrics.countByServiceName("zoidberg", true)
	metrics.countByServiceName("bender", true)
	metrics.countByServiceName("leela", true)
	metrics.countByServiceName("fry", true)

	counters, _ = baseMetrics.LocalBackend.Snapshot()
	assert.EqualValues(t, 1, counters["zoidberg|debug=true"])
	assert.EqualValues(t, 1, counters["bender|debug=true"])
	assert.EqualValues(t, 2, counters["other-services|debug=true"])
}
