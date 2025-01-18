// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	cFlags "github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	"github.com/jaegertracing/jaeger/cmd/collector/app/handler"
	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	zipkinsanitizer "github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer/zipkin"
	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/storage_v2/tracestore/mocks"
	"github.com/jaegertracing/jaeger/storage_v2/v1adapter"
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
							debug:       debug,
						})
				}
			}
		}
	}

	testFn := func(t *testing.T, test TestCase) {
		mb := metricstest.NewFactory(time.Hour)
		defer mb.Backend.Stop()
		logger := zap.NewNop()
		serviceMetrics := mb.Namespace(metrics.NSOptions{Name: "service", Tags: nil})
		hostMetrics := mb.Namespace(metrics.NSOptions{Name: "host", Tags: nil})
		sp, err := newSpanProcessor(
			v1adapter.NewTraceWriter(&fakeSpanWriter{}),
			nil,
			Options.ServiceMetrics(serviceMetrics),
			Options.HostMetrics(hostMetrics),
			Options.Logger(logger),
			Options.QueueSize(0),
			Options.BlockingSubmit(false),
			Options.ReportBusy(false),
			Options.SpanFilter(isSpanAllowed),
		)
		require.NoError(t, err)
		var metricPrefix, format string
		switch test.format {
		case processor.ZipkinSpanFormat:
			span := makeZipkinSpan(test.serviceName, test.rootSpan, test.debug)
			sanitizer := zipkinsanitizer.NewChainedSanitizer(zipkinsanitizer.NewStandardSanitizers()...)
			zHandler := handler.NewZipkinSpanHandler(logger, sp, sanitizer)
			zHandler.SubmitZipkinBatch(context.Background(), []*zc.Span{span, span}, handler.SubmitBatchOptions{})
			metricPrefix = "service"
			format = "zipkin"
		case processor.JaegerSpanFormat:
			span, process := makeJaegerSpan(test.serviceName, test.rootSpan, test.debug)
			jHandler := handler.NewJaegerSpanHandler(logger, sp)
			jHandler.SubmitBatches(context.Background(), []*jaeger.Batch{
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
					Name: metricPrefix + ".traces.received|debug=true|format=" + format + "|sampler_type=unrecognized|svc=" + test.serviceName + "|transport=unknown", Value: 2,
				})
			} else {
				expected = append(expected, metricstest.ExpectedMetric{
					Name: metricPrefix + ".traces.received|debug=false|format=" + format + "|sampler_type=unrecognized|svc=" + test.serviceName + "|transport=unknown", Value: 2,
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
	for _, test := range tests {
		t.Run(fmt.Sprintf("%v", test), func(t *testing.T) {
			testFn(t, test)
		})
	}
}

func isSpanAllowed(span *model.Span) bool {
	if span.Flags.IsDebug() {
		return true
	}

	return span.Process.ServiceName != blackListedService
}

type fakeSpanWriter struct {
	t         *testing.T
	spansLock sync.Mutex
	spans     []*model.Span
	err       error
	tenants   map[string]bool
}

func (n *fakeSpanWriter) WriteSpan(ctx context.Context, span *model.Span) error {
	if n.t != nil {
		n.t.Logf("Capturing span %+v", span)
	}
	n.spansLock.Lock()
	defer n.spansLock.Unlock()
	n.spans = append(n.spans, span)

	// Record all unique tenants arriving in span Contexts
	if n.tenants == nil {
		n.tenants = make(map[string]bool)
	}

	n.tenants[tenancy.GetTenant(ctx)] = true

	return n.err
}

func (*fakeSpanWriter) Close() error {
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
	p, err := NewSpanProcessor(v1adapter.NewTraceWriter(w), nil, Options.QueueSize(1))
	require.NoError(t, err)

	res, err := p.ProcessSpans(
		context.Background(),
		processor.SpansV1{
			Spans: []*model.Span{{}}, // empty span should be enriched by sanitizers
			Details: processor.Details{
				SpanFormat: processor.JaegerSpanFormat,
			},
		})
	require.NoError(t, err)
	assert.Equal(t, []bool{true}, res)
	require.NoError(t, p.Close())
	assert.Len(t, w.spans, 1)
	assert.NotNil(t, w.spans[0].Process)
	assert.NotEmpty(t, w.spans[0].Process.ServiceName)
}

func TestSpanProcessorErrors(t *testing.T) {
	logger, logBuf := testutils.NewLogger()
	w := &fakeSpanWriter{
		err: errors.New("some-error"),
	}
	mb := metricstest.NewFactory(time.Hour)
	defer mb.Backend.Stop()
	serviceMetrics := mb.Namespace(metrics.NSOptions{Name: "service", Tags: nil})
	pp, err := NewSpanProcessor(
		v1adapter.NewTraceWriter(w),
		nil,
		Options.Logger(logger),
		Options.ServiceMetrics(serviceMetrics),
		Options.QueueSize(1),
	)
	require.NoError(t, err)
	p := pp.(*spanProcessor)

	res, err := p.ProcessSpans(context.Background(), processor.SpansV1{
		Spans: []*model.Span{
			{
				Process: &model.Process{
					ServiceName: "x",
				},
			},
		},
		Details: processor.Details{
			SpanFormat: processor.JaegerSpanFormat,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, []bool{true}, res)

	require.NoError(t, p.Close())

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
	inWriteSpan atomic.Int32
}

func (w *blockingWriter) WriteSpan(context.Context, *model.Span) error {
	w.inWriteSpan.Add(1)
	w.Lock()
	defer w.Unlock()
	w.inWriteSpan.Add(-1)
	return nil
}

func TestSpanProcessorBusy(t *testing.T) {
	w := &blockingWriter{}
	pp, err := NewSpanProcessor(
		v1adapter.NewTraceWriter(w),
		nil,
		Options.NumWorkers(1),
		Options.QueueSize(1),
		Options.ReportBusy(true),
	)
	require.NoError(t, err)
	p := pp.(*spanProcessor)
	defer require.NoError(t, p.Close())

	// block the writer so that the first span is read from the queue and blocks the processor,
	// and either the second or the third span is rejected since the queue capacity is just 1.
	w.Lock()
	defer w.Unlock()

	res, err := p.ProcessSpans(context.Background(), processor.SpansV1{
		Spans: []*model.Span{
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
		},
		Details: processor.Details{
			SpanFormat: processor.JaegerSpanFormat,
		},
	})

	require.Error(t, err, "expecting busy error")
	assert.Nil(t, res)
}

func TestSpanProcessorWithNilProcess(t *testing.T) {
	mb := metricstest.NewFactory(time.Hour)
	defer mb.Backend.Stop()
	serviceMetrics := mb.Namespace(metrics.NSOptions{Name: "service", Tags: nil})

	w := &fakeSpanWriter{}
	pp, err := NewSpanProcessor(v1adapter.NewTraceWriter(w), nil, Options.ServiceMetrics(serviceMetrics))
	require.NoError(t, err)
	p := pp.(*spanProcessor)
	defer require.NoError(t, p.Close())

	p.saveSpan(&model.Span{}, "")

	expected := []metricstest.ExpectedMetric{{
		Name: "service.spans.saved-by-svc|debug=false|result=err|svc=__unknown", Value: 1,
	}}
	mb.AssertCounterMetrics(t, expected...)
}

func TestSpanProcessorWithCollectorTags(t *testing.T) {
	for _, modelVersion := range []string{"v1", "v2"} {
		t.Run(modelVersion, func(t *testing.T) {
			testCollectorTags := map[string]string{
				"extra": "tag",
				"env":   "prod",
				"node":  "172.22.18.161",
			}

			w := &fakeSpanWriter{}

			pp, err := NewSpanProcessor(
				v1adapter.NewTraceWriter(w),
				nil,
				Options.CollectorTags(testCollectorTags),
				Options.NumWorkers(1),
				Options.QueueSize(1),
			)
			require.NoError(t, err)
			p := pp.(*spanProcessor)
			t.Cleanup(func() {
				require.NoError(t, p.Close())
			})

			span := &model.Span{
				Process: model.NewProcess("unit-test-service", []model.KeyValue{
					model.String("env", "prod"),
					model.String("node", "k8s-test-node-01"),
				}),
			}

			var batch processor.Batch
			if modelVersion == "v2" {
				batch = processor.SpansV2{
					Traces: v1adapter.V1BatchesToTraces([]*model.Batch{{Spans: []*model.Span{span}}}),
				}
			} else {
				batch = processor.SpansV1{
					Spans: []*model.Span{span},
				}
			}
			_, err = p.ProcessSpans(context.Background(), batch)
			require.NoError(t, err)

			require.Eventually(t, func() bool {
				w.spansLock.Lock()
				defer w.spansLock.Unlock()
				return len(w.spans) > 0
			}, time.Second, time.Millisecond)

			w.spansLock.Lock()
			defer w.spansLock.Unlock()
			span = w.spans[0]

			expected := &model.Span{
				Process: model.NewProcess("unit-test-service", []model.KeyValue{
					model.String("env", "prod"),
					model.String("extra", "tag"),
					model.String("node", "172.22.18.161"),
					model.String("node", "k8s-test-node-01"),
				}),
			}
			if modelVersion == "v2" {
				// ptrace.Resource.Attributes do not allow duplicate keys,
				// so we only add non-conflicting tags, meaning the node IP
				// tag from the collectorTags will not be added.
				expected.Process.Tags = slices.Delete(expected.Process.Tags, 2, 3)
				typedTags := model.KeyValues(span.Process.Tags)
				typedTags.Sort()
			}

			m := &jsonpb.Marshaler{Indent: "  "}
			jsonActual := new(bytes.Buffer)
			m.Marshal(jsonActual, span.Process)
			jsonExpected := new(bytes.Buffer)
			m.Marshal(jsonExpected, expected.Process)
			assert.Equal(t, jsonExpected.String(), jsonActual.String())
		})
	}
}

func TestSpanProcessorCountSpan(t *testing.T) {
	tests := []struct {
		name                  string
		enableDynQueueSizeMem bool
		enableSpanMetrics     bool
		expectedUpdateGauge   bool
	}{
		{
			name:                  "enable dyn-queue-size, enable metrics",
			enableDynQueueSizeMem: true,
			enableSpanMetrics:     true,
			expectedUpdateGauge:   true,
		},
		{
			name:                  "enable dyn-queue-size, disable metrics",
			enableDynQueueSizeMem: true,
			enableSpanMetrics:     false,
			expectedUpdateGauge:   true,
		},
		{
			name:                  "disable dyn-queue-size, enable metrics",
			enableDynQueueSizeMem: false,
			enableSpanMetrics:     true,
			expectedUpdateGauge:   true,
		},
		{
			name:                  "disable dyn-queue-size, disable metrics",
			enableDynQueueSizeMem: false,
			enableSpanMetrics:     false,
			expectedUpdateGauge:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			mb := metricstest.NewFactory(time.Hour)
			defer mb.Backend.Stop()
			m := mb.Namespace(metrics.NSOptions{})

			w := &fakeSpanWriter{}
			opts := []Option{
				Options.HostMetrics(m),
				Options.SpanSizeMetricsEnabled(tt.enableSpanMetrics),
			}
			if tt.enableDynQueueSizeMem {
				opts = append(opts, Options.DynQueueSizeMemory(1000))
			} else {
				opts = append(opts, Options.DynQueueSizeMemory(0))
			}
			pp, err := NewSpanProcessor(v1adapter.NewTraceWriter(w), nil, opts...)
			require.NoError(t, err)
			p := pp.(*spanProcessor)
			defer func() {
				require.NoError(t, p.Close())
			}()
			p.background(10*time.Millisecond, p.updateGauges)

			p.processSpan(&model.Span{}, "")
			if tt.enableSpanMetrics {
				assert.Eventually(t,
					func() bool { return p.spansProcessed.Load() > 0 },
					time.Second,
					time.Millisecond,
				)
				assert.Positive(t, p.spansProcessed.Load())
			}

			for i := 0; i < 10000; i++ {
				_, g := mb.Snapshot()
				if b := g["spans.bytes"]; b > 0 {
					if !tt.expectedUpdateGauge {
						assert.Fail(t, "gauge has been updated unexpectedly")
					}
					assert.Equal(t, p.bytesProcessed.Load(), uint64(g["spans.bytes"]))
					return
				}
				time.Sleep(time.Millisecond)
			}

			if tt.expectedUpdateGauge {
				assert.Fail(t, "gauge hasn't been updated within a reasonable amount of time")
			}
		})
	}
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
			p, err := newSpanProcessor(v1adapter.NewTraceWriter(w), nil, Options.QueueSize(tt.initialCapacity), Options.DynQueueSizeWarmup(tt.warmup), Options.DynQueueSizeMemory(tt.sizeInBytes))
			require.NoError(t, err)
			assert.EqualValues(t, tt.initialCapacity, p.queue.Capacity())

			p.spansProcessed.Store(tt.spansProcessed)
			p.bytesProcessed.Store(tt.bytesProcessed)

			p.updateQueueSize()
			assert.EqualValues(t, tt.expectedCapacity, p.queue.Capacity())
		})
	}
}

func TestUpdateQueueSizeNoActivityYet(t *testing.T) {
	w := &fakeSpanWriter{}
	p, err := newSpanProcessor(v1adapter.NewTraceWriter(w), nil, Options.QueueSize(1), Options.DynQueueSizeWarmup(1), Options.DynQueueSizeMemory(1))
	require.NoError(t, err)
	assert.NotPanics(t, p.updateQueueSize)
}

func TestStartDynQueueSizeUpdater(t *testing.T) {
	w := &fakeSpanWriter{}
	oneGiB := uint(1024 * 1024 * 1024)

	p, err := newSpanProcessor(v1adapter.NewTraceWriter(w), nil, Options.QueueSize(100), Options.DynQueueSizeWarmup(1000), Options.DynQueueSizeMemory(oneGiB))
	require.NoError(t, err)
	assert.EqualValues(t, 100, p.queue.Capacity())

	p.spansProcessed.Store(1000)
	p.bytesProcessed.Store(10 * 1024 * p.spansProcessed.Load()) // 10KiB per span

	// 1024 ^ 3 / (10 * 1024) = 104857,6
	// ideal queue size = 104857
	p.background(10*time.Millisecond, p.updateQueueSize)

	// we wait up to 50 milliseconds
	for i := 0; i < 5; i++ {
		if p.queue.Capacity() != 100 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	assert.EqualValues(t, 104857, p.queue.Capacity())
	require.NoError(t, p.Close())
}

func TestAdditionalProcessors(t *testing.T) {
	w := &fakeSpanWriter{}

	// nil doesn't fail
	p, err := NewSpanProcessor(v1adapter.NewTraceWriter(w), nil, Options.QueueSize(1))
	require.NoError(t, err)
	res, err := p.ProcessSpans(context.Background(), processor.SpansV1{
		Spans: []*model.Span{
			{
				Process: &model.Process{
					ServiceName: "x",
				},
			},
		},
		Details: processor.Details{
			SpanFormat: processor.JaegerSpanFormat,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, []bool{true}, res)
	require.NoError(t, p.Close())

	// additional processor is called
	count := 0
	f := func(_ *model.Span, _ string) {
		count++
	}
	p, err = NewSpanProcessor(v1adapter.NewTraceWriter(w), []ProcessSpan{f}, Options.QueueSize(1))
	require.NoError(t, err)
	res, err = p.ProcessSpans(context.Background(), processor.SpansV1{
		Spans: []*model.Span{
			{
				Process: &model.Process{
					ServiceName: "x",
				},
			},
		},
		Details: processor.Details{
			SpanFormat: processor.JaegerSpanFormat,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, []bool{true}, res)
	require.NoError(t, p.Close())
	assert.Equal(t, 1, count)
}

func TestSpanProcessorContextPropagation(t *testing.T) {
	w := &fakeSpanWriter{}
	p, err := NewSpanProcessor(v1adapter.NewTraceWriter(w), nil, Options.QueueSize(1))
	require.NoError(t, err)

	dummyTenant := "context-prop-test-tenant"

	res, err := p.ProcessSpans(context.Background(), processor.SpansV1{
		Spans: []*model.Span{
			{
				Process: &model.Process{
					ServiceName: "x",
				},
			},
		},
		Details: processor.Details{
			Tenant: dummyTenant,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, []bool{true}, res)
	require.NoError(t, p.Close())

	// Verify that the dummy tenant from SpansOptions context made it to writer
	assert.True(t, w.tenants[dummyTenant])
	// Verify no other tenantKey context values made it to writer
	assert.True(t, reflect.DeepEqual(w.tenants, map[string]bool{dummyTenant: true}))
}

func TestSpanProcessorWithOnDroppedSpanOption(t *testing.T) {
	var droppedOperations []string
	customOnDroppedSpan := func(span *model.Span) {
		droppedOperations = append(droppedOperations, span.OperationName)
	}

	w := &blockingWriter{}
	pp, err := NewSpanProcessor(
		v1adapter.NewTraceWriter(w),
		nil,
		Options.NumWorkers(1),
		Options.QueueSize(1),
		Options.OnDroppedSpan(customOnDroppedSpan),
		Options.ReportBusy(true),
	)
	require.NoError(t, err)
	p := pp.(*spanProcessor)
	defer p.Close()

	// Acquire the lock externally to force the writer to block.
	w.Lock()
	defer w.Unlock()

	_, err = p.ProcessSpans(context.Background(), processor.SpansV1{
		Spans: []*model.Span{
			{OperationName: "op1"},
		},
		Details: processor.Details{
			SpanFormat: processor.JaegerSpanFormat,
		},
	})
	require.NoError(t, err)

	// Wait for the sole worker to pick the item from the queue and block
	assert.Eventually(t,
		func() bool { return w.inWriteSpan.Load() == 1 },
		time.Second, time.Microsecond)

	// Now the queue is empty again and can accept one more item, but no workers available.
	// If we send two items, the last one will have to be dropped.
	_, err = p.ProcessSpans(context.Background(), processor.SpansV1{
		Spans: []*model.Span{
			{OperationName: "op2"},
			{OperationName: "op3"},
		},
		Details: processor.Details{
			SpanFormat: processor.JaegerSpanFormat,
		},
	})
	require.EqualError(t, err, processor.ErrBusy.Error())
	assert.Equal(t, []string{"op3"}, droppedOperations)
}

func optionsWithPorts(portHttp string, portGrpc string) *cFlags.CollectorOptions {
	opts := &cFlags.CollectorOptions{
		OTLP: struct {
			Enabled bool
			GRPC    configgrpc.ServerConfig
			HTTP    confighttp.ServerConfig
		}{
			Enabled: true,
			HTTP: confighttp.ServerConfig{
				Endpoint: portHttp,
			},
			GRPC: configgrpc.ServerConfig{
				NetAddr: confignet.AddrConfig{
					Endpoint:  portGrpc,
					Transport: confignet.TransportTypeTCP,
				},
			},
		},
	}
	return opts
}

func TestOTLPReceiverWithV2Storage(t *testing.T) {
	// Setup mock writer and expectations
	mockWriter := mocks.NewWriter(t)
	traces := ptrace.NewTraces()
	rSpans := traces.ResourceSpans().AppendEmpty()
	sSpans := rSpans.ScopeSpans().AppendEmpty()
	span := sSpans.Spans().AppendEmpty()
	span.SetName("test-trace")

	var receivedTraces atomic.Pointer[ptrace.Traces]
	mockWriter.On("WriteTraces", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			storeTrace := args.Get(1).(ptrace.Traces)
			receivedTraces.Store(&storeTrace)
		}).Return(nil)

	spanProcessor, err := NewSpanProcessor(
		mockWriter,
		nil,
		Options.NumWorkers(1),
		Options.QueueSize(1),
		Options.ReportBusy(true),
	)
	require.NoError(t, err)
	defer spanProcessor.Close()
	logger := zaptest.NewLogger(t)

	portHttp := "4318"
	portGrpc := "4317"

	// Can't send tenancy headers with http request to OTLP receiver
	tenancyMgr := &tenancy.Manager{
		Enabled: false,
	}

	// Create and start receiver
	rec, err := handler.StartOTLPReceiver(
		optionsWithPorts(fmt.Sprintf("localhost:%v", portHttp), fmt.Sprintf("localhost:%v", portGrpc)),
		logger,
		spanProcessor,
		tenancyMgr,
	)
	require.NoError(t, err)
	ctx := context.Background()
	defer rec.Shutdown(ctx)

	// Send trace via HTTP
	url := fmt.Sprintf("http://localhost:%v/v1/traces", portHttp)
	client := &http.Client{}

	marshaler := ptrace.JSONMarshaler{}
	data, err := marshaler.MarshalTraces(traces)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	require.NoError(t, err)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("x-tenant", "test-tenant")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	time.Sleep(100 * time.Millisecond)

	receivedSpan := receivedTraces.Load().ResourceSpans().At(0).
		ScopeSpans().At(0).
		Spans().At(0)
	require.Equal(t, span.Name(), receivedSpan.Name())

	mockWriter.AssertExpectations(t)
}
