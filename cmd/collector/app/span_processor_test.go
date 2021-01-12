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
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger-lib/metrics/metricstest"
	"go.uber.org/atomic"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/handler"
	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	zipkinSanitizer "github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer/zipkin"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	zc "github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

var (
	_ io.Closer = (*fakeSpanWriter)(nil)
	_ io.Closer = (*spanProcessor)(nil)

	blackListedService = "zoidberg"
)

func TestBySvcMetrics(t *testing.T) {
	allowedService := "bender"

	type TestCase struct {
		format      processor.SpanFormat
		serviceName string
		rootSpan    bool
		debug       bool
	}

	spanFormat := [2]processor.SpanFormat{processor.ZipkinSpanFormat, processor.JaegerSpanFormat}
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
		sp := newSpanProcessor(
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
		case processor.ZipkinSpanFormat:
			span := makeZipkinSpan(test.serviceName, test.rootSpan, test.debug)
			zHandler := handler.NewZipkinSpanHandler(logger, sp, zipkinSanitizer.NewParentIDSanitizer())
			zHandler.SubmitZipkinBatch([]*zc.Span{span, span}, handler.SubmitBatchOptions{})
			metricPrefix = "service"
			format = "zipkin"
		case processor.JaegerSpanFormat:
			span, process := makeJaegerSpan(test.serviceName, test.rootSpan, test.debug)
			jHandler := handler.NewJaegerSpanHandler(logger, sp)
			jHandler.SubmitBatches([]*jaeger.Batch{
				{
					Spans: []*jaeger.Span{
						span,
						span,
					},
					Process: process,
				},
			}, handler.SubmitBatchOptions{})
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

func (n *fakeSpanWriter) WriteSpan(ctx context.Context, span *model.Span) error {
	return n.err
}

func (n *fakeSpanWriter) Close() error {
	return nil
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
	p := NewSpanProcessor(w, Options.QueueSize(1)).(*spanProcessor)

	res, err := p.ProcessSpans([]*model.Span{
		{
			Process: &model.Process{
				ServiceName: "x",
			},
		},
	}, processor.SpansOptions{SpanFormat: processor.JaegerSpanFormat})
	assert.NoError(t, err)
	assert.Equal(t, []bool{true}, res)
	assert.NoError(t, p.Close())
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
		Options.QueueSize(1),
	).(*spanProcessor)

	res, err := p.ProcessSpans([]*model.Span{
		{
			Process: &model.Process{
				ServiceName: "x",
			},
		},
	}, processor.SpansOptions{SpanFormat: processor.JaegerSpanFormat})
	assert.NoError(t, err)
	assert.Equal(t, []bool{true}, res)

	assert.NoError(t, p.Close())

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

func (w *blockingWriter) WriteSpan(ctx context.Context, span *model.Span) error {
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
	defer assert.NoError(t, p.Close())

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
	}, processor.SpansOptions{SpanFormat: processor.JaegerSpanFormat})

	assert.Error(t, err, "expcting busy error")
	assert.Nil(t, res)
}

func TestSpanProcessorWithNilProcess(t *testing.T) {
	mb := metricstest.NewFactory(time.Hour)
	serviceMetrics := mb.Namespace(metrics.NSOptions{Name: "service", Tags: nil})

	w := &fakeSpanWriter{}
	p := NewSpanProcessor(w, Options.ServiceMetrics(serviceMetrics)).(*spanProcessor)
	defer assert.NoError(t, p.Close())

	p.saveSpan(&model.Span{})

	expected := []metricstest.ExpectedMetric{{
		Name: "service.spans.saved-by-svc|debug=false|result=err|svc=__unknown", Value: 1,
	}}
	mb.AssertCounterMetrics(t, expected...)
}

func TestSpanProcessorWithCollectorTags(t *testing.T) {
	testCollectorTags := map[string]string{
		"extra": "tag",
		"env":   "prod",
		"node":  "172.22.18.161",
	}

	w := &fakeSpanWriter{}
	p := NewSpanProcessor(w, Options.CollectorTags(testCollectorTags)).(*spanProcessor)

	defer assert.NoError(t, p.Close())
	span := &model.Span{
		Process: model.NewProcess("unit-test-service", []model.KeyValue{
			{
				Key:  "env",
				VStr: "prod",
			},
			{
				Key:  "node",
				VStr: "k8s-test-node-01",
			},
		}),
	}
	p.addCollectorTags(span)
	expected := &model.Span{
		Process: model.NewProcess("unit-test-service", []model.KeyValue{
			{
				Key:  "env",
				VStr: "prod",
			},
			{
				Key:  "extra",
				VStr: "tag",
			},
			{
				Key:  "node",
				VStr: "172.22.18.161",
			},
			{
				Key:  "node",
				VStr: "k8s-test-node-01",
			},
		}),
	}

	assert.Equal(t, expected.Process, span.Process)
}

func TestSpanProcessorCountSpan(t *testing.T) {
	mb := metricstest.NewFactory(time.Hour)
	m := mb.Namespace(metrics.NSOptions{})

	w := &fakeSpanWriter{}
	p := NewSpanProcessor(w, Options.HostMetrics(m), Options.DynQueueSizeMemory(1000)).(*spanProcessor)
	p.background(10*time.Millisecond, p.updateGauges)

	p.processSpan(&model.Span{})
	assert.NotEqual(t, uint64(0), p.bytesProcessed)

	for i := 0; i < 15; i++ {
		_, g := mb.Snapshot()
		if b := g["spans.bytes"]; b > 0 {
			assert.Equal(t, p.bytesProcessed.Load(), uint64(g["spans.bytes"]))
			return
		}
		time.Sleep(time.Millisecond)
	}

	assert.Fail(t, "gauge hasn't been updated within a reasonable amount of time")
	assert.NoError(t, p.Close())
}

func TestUpdateDynQueueSize(t *testing.T) {
	tests := []struct {
		name             string
		sizeInBytes      uint
		initialCapacity  int
		warmup           uint
		spansProcessed   uint64
		bytesProcessed   uint64
		expectedCapacity int
	}{
		{
			name:             "scale-up",
			sizeInBytes:      uint(1024 * 1024 * 1024), // one GiB
			initialCapacity:  100,
			warmup:           1000,
			spansProcessed:   uint64(1000),
			bytesProcessed:   uint64(10 * 1024 * 1000), // 10KiB per span
			expectedCapacity: 104857,                   // 1024 ^ 3 / (10 * 1024) = 104857,6
		},
		{
			name:             "scale-down",
			sizeInBytes:      uint(1024 * 1024), // one MiB
			initialCapacity:  1000,
			warmup:           1000,
			spansProcessed:   uint64(1000),
			bytesProcessed:   uint64(10 * 1024 * 1000),
			expectedCapacity: 102, // 1024 ^ 2 / (10 * 1024) = 102,4
		},
		{
			name:             "not-enough-change",
			sizeInBytes:      uint(1024 * 1024),
			initialCapacity:  100,
			warmup:           1000,
			spansProcessed:   uint64(1000),
			bytesProcessed:   uint64(10 * 1024 * 1000),
			expectedCapacity: 100, // 1024 ^ 2 / (10 * 1024) = 102,4, 2% change only
		},
		{
			name:             "not-enough-spans",
			sizeInBytes:      uint(1024 * 1024 * 1024),
			initialCapacity:  100,
			warmup:           1000,
			spansProcessed:   uint64(999),
			bytesProcessed:   uint64(10 * 1024 * 1000),
			expectedCapacity: 100,
		},
		{
			name:             "not-enabled",
			sizeInBytes:      uint(1024 * 1024 * 1024), // one GiB
			initialCapacity:  100,
			warmup:           0,
			spansProcessed:   uint64(1000),
			bytesProcessed:   uint64(10 * 1024 * 1000), // 10KiB per span
			expectedCapacity: 100,
		},
		{
			name:             "memory-not-set",
			sizeInBytes:      0,
			initialCapacity:  100,
			warmup:           1000,
			spansProcessed:   uint64(1000),
			bytesProcessed:   uint64(10 * 1024 * 1000), // 10KiB per span
			expectedCapacity: 100,
		},
		{
			name:             "max-queue-size",
			sizeInBytes:      uint(10 * 1024 * 1024 * 1024),
			initialCapacity:  100,
			warmup:           1000,
			spansProcessed:   uint64(1000),
			bytesProcessed:   uint64(10 * 1024 * 1000), // 10KiB per span
			expectedCapacity: maxQueueSize,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &fakeSpanWriter{}
			p := newSpanProcessor(w, Options.QueueSize(tt.initialCapacity), Options.DynQueueSizeWarmup(tt.warmup), Options.DynQueueSizeMemory(tt.sizeInBytes))
			assert.EqualValues(t, tt.initialCapacity, p.queue.Capacity())

			p.spansProcessed = atomic.NewUint64(tt.spansProcessed)
			p.bytesProcessed = atomic.NewUint64(tt.bytesProcessed)

			p.updateQueueSize()
			assert.EqualValues(t, tt.expectedCapacity, p.queue.Capacity())
		})
	}
}

func TestUpdateQueueSizeNoActivityYet(t *testing.T) {
	w := &fakeSpanWriter{}
	p := newSpanProcessor(w, Options.QueueSize(1), Options.DynQueueSizeWarmup(1), Options.DynQueueSizeMemory(1))
	assert.NotPanics(t, p.updateQueueSize)
}

func TestStartDynQueueSizeUpdater(t *testing.T) {
	w := &fakeSpanWriter{}
	oneGiB := uint(1024 * 1024 * 1024)
	p := newSpanProcessor(w, Options.QueueSize(100), Options.DynQueueSizeWarmup(1000), Options.DynQueueSizeMemory(oneGiB))
	assert.EqualValues(t, 100, p.queue.Capacity())

	p.spansProcessed = atomic.NewUint64(1000)
	p.bytesProcessed = atomic.NewUint64(10 * 1024 * p.spansProcessed.Load()) // 10KiB per span

	// 1024 ^ 3 / (10 * 1024) = 104857,6
	// ideal queue size = 104857
	p.background(10*time.Millisecond, p.updateQueueSize)

	// we wait up to 50 milliseconds
	for i := 0; i < 5; i++ {
		if p.queue.Capacity() == 100 {
			time.Sleep(10 * time.Millisecond)
		} else {
			break
		}
	}

	assert.EqualValues(t, 104857, p.queue.Capacity())
}
