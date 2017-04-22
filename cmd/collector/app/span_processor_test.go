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
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger-lib/metrics"
	metricsTest "github.com/uber/jaeger-lib/metrics/testutils"
	"github.com/uber/tchannel-go/thrift"
	"go.uber.org/zap"
	"golang.org/x/net/context"

	zipkinSanitizer "github.com/uber/jaeger/cmd/collector/app/sanitizer/zipkin"
	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/pkg/testutils"
	"github.com/uber/jaeger/thrift-gen/jaeger"
	zc "github.com/uber/jaeger/thrift-gen/zipkincore"
)

var blackListedService = "zoidberg"

func TestBySvcMetrics(t *testing.T) {
	allowedService := "bender"

	type TestCase struct {
		format      string
		serviceName string
		rootSpan    bool
		debug       bool
	}

	spanFormat := [2]string{ZipkinFormatType, JaegerFormatType}
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
		mb := metrics.NewLocalFactory(time.Hour)
		logger := zap.NewNop()
		serviceMetrics := mb.Namespace("service", nil)
		hostMetrics := mb.Namespace("host", nil)
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
		ctx := context.Background()
		tctx := thrift.Wrap(ctx)
		var metricPrefix string
		if test.format == ZipkinFormatType {
			span := makeZipkinSpan(test.serviceName, test.rootSpan, test.debug)
			zHandler := NewZipkinSpanHandler(logger, processor, zipkinSanitizer.NewParentIDSanitizer(logger))
			zHandler.SubmitZipkinBatch(tctx, []*zc.Span{span, span})
			metricPrefix = "service.zipkin"
		} else if test.format == JaegerFormatType {
			span, process := makeJaegerSpan(test.serviceName, test.rootSpan, test.debug)
			jHandler := NewJaegerSpanHandler(logger, processor)
			jHandler.SubmitBatches(tctx, []*jaeger.Batch{
				{
					Spans: []*jaeger.Span{
						span,
						span,
					},
					Process: process,
				},
			})
			metricPrefix = "service.jaeger"
		} else {
			panic("Unknown format")
		}
		expected := []metricsTest.ExpectedMetric{
			{Name: metricPrefix + ".spans.recd", Value: 2},
			{Name: metricPrefix + ".spans.by-svc." + test.serviceName, Value: 2},
		}
		if test.debug {
			expected = append(expected, metricsTest.ExpectedMetric{
				Name: metricPrefix + ".debug-spans.by-svc." + test.serviceName, Value: 2,
			})
		}
		if test.rootSpan {
			expected = append(expected, metricsTest.ExpectedMetric{
				Name: metricPrefix + ".traces.by-svc." + test.serviceName, Value: 2,
			})
		}
		if test.serviceName != blackListedService || test.debug {
			// "error.busy" and "spans.dropped" are both equivalent to a span being accepted,
			// because both are emitted when attempting to add span to the queue, and since
			// we defined the queue capacity as 0, all submitted items are dropped.
			// The debug spans are always accepted.
			expected = append(expected, metricsTest.ExpectedMetric{
				Name: "host.error.busy", Value: 2,
			}, metricsTest.ExpectedMetric{
				Name: "host.spans.dropped", Value: 2,
			})
		} else {
			expected = append(expected, metricsTest.ExpectedMetric{
				Name: metricPrefix + ".spans.rejected", Value: 2,
			})
		}
		metricsTest.AssertCounterMetrics(t, mb, expected...)
	}
}

func isSpanAllowed(span *model.Span) bool {
	if span.Flags.IsDebug() {
		return true
	}

	serviceName := span.Process.ServiceName
	if serviceName == blackListedService {
		return false
	}
	return true
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
		&model.Span{
			Process: &model.Process{
				ServiceName: "x",
			},
		},
	}, JaegerFormatType)
	assert.NoError(t, err)
	assert.Equal(t, []bool{true}, res)
}

func TestSpanProcessorErrors(t *testing.T) {
	logger, logBuf := testutils.NewLogger()
	w := &fakeSpanWriter{
		err: fmt.Errorf("some-error"),
	}
	p := NewSpanProcessor(w,
		Options.Logger(logger),
	).(*spanProcessor)

	res, err := p.ProcessSpans([]*model.Span{
		&model.Span{
			Process: &model.Process{
				ServiceName: "x",
			},
		},
	}, JaegerFormatType)

	assert.NoError(t, err)
	assert.Equal(t, []bool{true}, res)

	p.Stop()

	assert.Equal(t, map[string]string{
		"level": "error",
		"msg":   "Failed to save span",
		"error": "some-error",
	}, logBuf.JSONLine(0))
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
		&model.Span{
			Process: &model.Process{
				ServiceName: "x",
			},
		},
		&model.Span{
			Process: &model.Process{
				ServiceName: "x",
			},
		},
		&model.Span{
			Process: &model.Process{
				ServiceName: "x",
			},
		},
	}, JaegerFormatType)

	assert.Error(t, err, "expcting busy error")
	assert.Nil(t, res)
}
