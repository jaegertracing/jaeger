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
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger-lib/metrics/metricstest"
	"go.uber.org/zap"

	zipkinSanitizer "github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer/zipkin"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	zc "github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

var blackListedService = "zoidberg"

func TestBySvcMetrics(t *testing.T) {
	allowedService := "bender"

	type TestCase struct {
		format      SpanFormat
		serviceName string
		rootSpan    bool
		debug       bool
	}

	spanFormat := [2]SpanFormat{ZipkinSpanFormat, JaegerSpanFormat}
	serviceNames := [2]string{allowedService, blackListedService}
	rootSpanEnabled := [2]bool{true, false}
	debugEnabled := [2]bool{true, false}

	// generate test cases as permutations of above parameters
	var tests []TestCase
	for _, format := range spanFormat {
		for _, serviceName := range serviceNames {
			for _, rootSpan := range rootSpanEnabled {
				for _, debug := range debugEnabled {
					tests = append(tests,
						TestCase{
							format:      format,
							serviceName: serviceName,
							rootSpan:    rootSpan,
							debug:       debug})
				}
			}
		}
	}

	for _, test := range tests {
		mb := metricstest.NewFactory(time.Hour)
		logger := zap.NewNop()
		serviceMetrics := mb.Namespace(metrics.NSOptions{Name: "service", Tags: nil})
		hostMetrics := mb.Namespace(metrics.NSOptions{Name: "host", Tags: nil})
		processor := newSpanProcessor(
			&fakeSpanWriter{},
			Options.ServiceMetrics(serviceMetrics),
			Options.HostMetrics(hostMetrics),
			Options.Logger(logger),
			Options.QueueSize(0),
			Options.BlockingSubmit(false),
			Options.ReportBusy(false),
			Options.SpanFilter(isSpanAllowed),
		)
		var metricPrefix, format string
		switch test.format {
		case ZipkinSpanFormat:
			span := makeZipkinSpan(test.serviceName, test.rootSpan, test.debug)
			zHandler := NewZipkinSpanHandler(logger, processor, zipkinSanitizer.NewParentIDSanitizer())
			zHandler.SubmitZipkinBatch([]*zc.Span{span, span}, SubmitBatchOptions{})
			metricPrefix = "service"
			format = "zipkin"
		case JaegerSpanFormat:
			span, process := makeJaegerSpan(test.serviceName, test.rootSpan, test.debug)
			jHandler := NewJaegerSpanHandler(logger, processor)
			jHandler.SubmitBatches([]*jaeger.Batch{
				{
					Spans: []*jaeger.Span{
						span,
						span,
					},
					Process: process,
				},
			}, SubmitBatchOptions{})
			metricPrefix = "service"
			format = "jaeger"
		default:
			panic("Unknown format")
		}
		expected := []metricstest.ExpectedMetric{}
		if test.debug {
			expected = append(expected, metricstest.ExpectedMetric{
				Name: metricPrefix + ".spans.received|debug=true|format=" + format + "|svc=" + test.serviceName + "|transport=unknown", Value: 2,
			})
		} else {
			expected = append(expected, metricstest.ExpectedMetric{
				Name: metricPrefix + ".spans.received|debug=false|format=" + format + "|svc=" + test.serviceName + "|transport=unknown", Value: 2,
			})
		}
		if test.rootSpan {
			if test.debug {
				expected = append(expected, metricstest.ExpectedMetric{
					Name: metricPrefix + ".traces.received|debug=true|format=" + format + "|sampler_type=unknown|svc=" + test.serviceName + "|transport=unknown", Value: 2,
				})
			} else {
				expected = append(expected, metricstest.ExpectedMetric{
					Name: metricPrefix + ".traces.received|debug=false|format=" + format + "|sampler_type=unknown|svc=" + test.serviceName + "|transport=unknown", Value: 2,
				})
			}
		}
		if test.serviceName != blackListedService || test.debug {
			// "error.busy" and "spans.dropped" are both equivalent to a span being accepted,
			// because both are emitted when attempting to add span to the queue, and since
			// we defined the queue capacity as 0, all submitted items are dropped.
			// The debug spans are always accepted.
			expected = append(expected, metricstest.ExpectedMetric{
				Name: "host.spans.dropped", Value: 2,
			})
		} else {
			expected = append(expected, metricstest.ExpectedMetric{
				Name: metricPrefix + ".spans.rejected|debug=false|format=" + format + "|svc=" + test.serviceName + "|transport=unknown", Value: 2,
			})
		}
		mb.AssertCounterMetrics(t, expected...)
	}
}

func isSpanAllowed(span *model.Span) bool {
	if span.Flags.IsDebug() {
		return true
	}

	return span.Process.ServiceName != blackListedService
}

type fakeSpanWriter struct {
	err error
}

func (n *fakeSpanWriter) WriteSpan(span *model.Span) error {
	return n.err
}

func makeZipkinSpan(service string, rootSpan bool, debugEnabled bool) *zc.Span {
	var parentID *int64
	if !rootSpan {
		p := int64(1)
		parentID = &p
	}
	span := &zc.Span{
		Name:     "zipkin",
		ParentID: parentID,
		Annotations: []*zc.Annotation{
			{
				Value: "cs",
				Host: &zc.Endpoint{
					ServiceName: service,
				},
			},
		},
		ID:    42,
		Debug: debugEnabled,
	}
	return span
}

func makeJaegerSpan(service string, rootSpan bool, debugEnabled bool) (*jaeger.Span, *jaeger.Process) {
	flags := int32(0)
	if debugEnabled {
		flags = 2
	}
	parentID := int64(0)
	if !rootSpan {
		parentID = int64(1)
	}
	return &jaeger.Span{
			OperationName: "jaeger",
			Flags:         flags,
			ParentSpanId:  parentID,
			TraceIdLow:    42,
		}, &jaeger.Process{
			ServiceName: service,
		}
}

func TestSpanProcessor(t *testing.T) {
	w := &fakeSpanWriter{}
	p := NewSpanProcessor(w).(*spanProcessor)
	defer p.Stop()

	res, err := p.ProcessSpans([]*model.Span{
		{
			Process: &model.Process{
				ServiceName: "x",
			},
		},
	}, ProcessSpansOptions{SpanFormat: JaegerSpanFormat})
	assert.NoError(t, err)
	assert.Equal(t, []bool{true}, res)
}

func TestSpanProcessorErrors(t *testing.T) {
	logger, logBuf := testutils.NewLogger()
	w := &fakeSpanWriter{
		err: fmt.Errorf("some-error"),
	}
	mb := metricstest.NewFactory(time.Hour)
	serviceMetrics := mb.Namespace(metrics.NSOptions{Name: "service", Tags: nil})
	p := NewSpanProcessor(w,
		Options.Logger(logger),
		Options.ServiceMetrics(serviceMetrics),
	).(*spanProcessor)

	res, err := p.ProcessSpans([]*model.Span{
		{
			Process: &model.Process{
				ServiceName: "x",
			},
		},
	}, ProcessSpansOptions{SpanFormat: JaegerSpanFormat})
	assert.NoError(t, err)
	assert.Equal(t, []bool{true}, res)

	p.Stop()

	assert.Equal(t, map[string]string{
		"level": "error",
		"msg":   "Failed to save span",
		"error": "some-error",
	}, logBuf.JSONLine(0))

	expected := []metricstest.ExpectedMetric{{
		Name: "service.spans.saved-by-svc|debug=false|result=err|svc=x", Value: 1,
	}}
	mb.AssertCounterMetrics(t, expected...)
}

type blockingWriter struct {
	sync.Mutex
}

func (w *blockingWriter) WriteSpan(span *model.Span) error {
	w.Lock()
	defer w.Unlock()
	return nil
}

func TestSpanProcessorBusy(t *testing.T) {
	w := &blockingWriter{}
	p := NewSpanProcessor(w,
		Options.NumWorkers(1),
		Options.QueueSize(1),
		Options.ReportBusy(true),
	).(*spanProcessor)
	defer p.Stop()

	// block the writer so that the first span is read from the queue and blocks the processor,
	// and eiher the second or the third span is rejected since the queue capacity is just 1.
	w.Lock()
	defer w.Unlock()

	res, err := p.ProcessSpans([]*model.Span{
		{
			Process: &model.Process{
				ServiceName: "x",
			},
		},
		{
			Process: &model.Process{
				ServiceName: "x",
			},
		},
		{
			Process: &model.Process{
				ServiceName: "x",
			},
		},
	}, ProcessSpansOptions{SpanFormat: JaegerSpanFormat})

	assert.Error(t, err, "expcting busy error")
	assert.Nil(t, res)
}

func TestSpanProcessorWithNilProcess(t *testing.T) {
	mb := metricstest.NewFactory(time.Hour)
	serviceMetrics := mb.Namespace(metrics.NSOptions{Name: "service", Tags: nil})

	w := &fakeSpanWriter{}
	p := NewSpanProcessor(w, Options.ServiceMetrics(serviceMetrics)).(*spanProcessor)
	defer p.Stop()

	p.saveSpan(&model.Span{})

	expected := []metricstest.ExpectedMetric{{
		Name: "service.spans.saved-by-svc|debug=false|result=err|svc=__unknown", Value: 1,
	}}
	mb.AssertCounterMetrics(t, expected...)
}

func TestSpanProcessorWithCollectorTags(t *testing.T) {

	testCollectorTags := map[string]string{
		"extra": "tag",
	}

	w := &fakeSpanWriter{}
	p := NewSpanProcessor(w, Options.CollectorTags(testCollectorTags)).(*spanProcessor)
	defer p.Stop()

	span := &model.Span{
		Process: model.NewProcess("unit-test-service", []model.KeyValue{}),
	}

	p.addCollectorTags(span)

	for k, v := range testCollectorTags {
		var foundTag bool
		for _, tag := range span.Process.Tags {
			if tag.GetKey() == k {
				assert.Equal(t, v, tag.AsStringLossy())
				foundTag = true
				break
			}
		}
		assert.True(t, foundTag)
	}
}
