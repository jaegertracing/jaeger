// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	jaegerM "github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger/model"
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
	mSpan.ParentSpanID = model.SpanID(1234)
	jFormat.ReceivedBySvc.ReportServiceNameForSpan(&mSpan)
	counters, gauges := baseMetrics.LocalBackend.Snapshot()

	assert.EqualValues(t, 2, counters["service.jaeger.spans.by-svc.fry"])
	assert.EqualValues(t, 1, counters["service.jaeger.traces.by-svc.fry"])
	assert.EqualValues(t, 1, counters["service.jaeger.debug-spans.by-svc.fry"])
	assert.Empty(t, gauges)
}
