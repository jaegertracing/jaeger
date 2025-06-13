// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracegen

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type worker struct {
	tracers []trace.Tracer
	running *uint32 // pointer to shared flag that indicates it's time to stop the test
	id      int     // worker id
	Config
	wg     *sync.WaitGroup // notify when done
	logger *zap.Logger

	// internal counters
	traceNo   int
	attrKeyNo int
	attrValNo int

	// for trace&span static time
	spanInterval int
}

const (
	fakeSpanDuration = 123 * time.Microsecond
)

func (w *worker) simulateTraces() {
	for atomic.LoadUint32(w.running) == 1 {
		svcNo := w.traceNo % len(w.tracers)
		w.simulateOneTrace(w.tracers[svcNo])
		w.traceNo++
		if w.Traces != 0 {
			if w.traceNo >= w.Traces {
				break
			}
		}
	}
	w.logger.Info(fmt.Sprintf("Worker %d generated %d traces", w.id, w.traceNo))
	w.wg.Done()
}

func (w *worker) simulateOneTrace(tracer trace.Tracer) {
	ctx := context.Background()
	attrs := []attribute.KeyValue{
		attribute.String("peer.service", "tracegen-server"),
		attribute.String("peer.host.ipv4", "1.1.1.1"),
	}
	if w.Debug {
		attrs = append(attrs, attribute.Bool("jaeger.debug", true))
	}
	if w.Firehose {
		attrs = append(attrs, attribute.Bool("jaeger.firehose", true))
	}
	start := time.Now()
	ctx, parent := tracer.Start(
		ctx,
		"lets-go",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(attrs...),
		trace.WithTimestamp(start),
	)
	w.simulateChildSpans(ctx, start, tracer)

	if w.Pause != 0 {
		parent.End()
	} else {
		totalDuration := time.Duration(w.ChildSpans) * fakeSpanDuration
		parent.End(
			trace.WithTimestamp(start.Add(totalDuration)),
		)
	}
}

func (w *worker) simulateChildSpans(ctx context.Context, start time.Time, tracer trace.Tracer) {
	for c := 0; c < w.ChildSpans; c++ {
		var attrs []attribute.KeyValue
		for a := 0; a < w.Attributes; a++ {
			key := fmt.Sprintf("attr_%02d", w.attrKeyNo)
			val := fmt.Sprintf("val_%02d", w.attrValNo)
			attrs = append(attrs, attribute.String(key, val))
			w.attrKeyNo = (w.attrKeyNo + 1) % w.AttrKeys
			w.attrValNo = (w.attrValNo + 1) % w.AttrValues
		}
		opts := []trace.SpanStartOption{
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(attrs...),
		}
		childStart := start.Add(time.Duration(c) * fakeSpanDuration)
		if w.Pause == 0 {
			opts = append(opts, trace.WithTimestamp(childStart))
		}
		_, child := tracer.Start(
			ctx,
			fmt.Sprintf("child-span-%02d", c),
			opts...,
		)
		if w.Pause != 0 {
			time.Sleep(w.Pause)
			child.End()
		} else {
			child.End(
				trace.WithTimestamp(childStart.Add(fakeSpanDuration)),
			)
		}
	}
}

// For static simulate
func (w *worker) simulateStaticTraces(startTime time.Time) {
	// reset
	w.spanInterval = 1
	w.traceNo = 0

	for atomic.LoadUint32(w.running) == 1 {
		svcNo := w.traceNo % len(w.tracers)
		w.simulateStaticOneTrace(w.tracers[svcNo], startTime)
		w.traceNo++
		if w.Traces != 0 && w.traceNo >= w.Traces {
			break
		}
	}
	w.logger.Info(fmt.Sprintf("Worker %d generated %d traces", w.id, w.traceNo))
	w.wg.Done()
}

func (w *worker) simulateStaticOneTrace(tracer trace.Tracer, baseStartTime time.Time) {
	ctx := context.Background()
	attrs := []attribute.KeyValue{
		attribute.String("peer.service", "tracegen-server"),
		attribute.String("peer.host.ipv4", "1.1.1.1"),
	}
	if w.Debug {
		attrs = append(attrs, attribute.Bool("jaeger.debug", true))
	}
	if w.Firehose {
		attrs = append(attrs, attribute.Bool("jaeger.firehose", true))
	}

	start := baseStartTime
	ctx, parent := tracer.Start(
		ctx,
		"lets-go",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(attrs...),
		trace.WithTimestamp(start),
	)

	lastEnd := w.simulateStaticChildSpans(ctx, start, tracer)

	parent.End(trace.WithTimestamp(lastEnd))
}

func (w *worker) simulateStaticChildSpans(ctx context.Context, baseStart time.Time, tracer trace.Tracer) time.Time {
	lastEnd := baseStart

	for c := 0; c < w.ChildSpans; c++ {
		spanStart := lastEnd

		if w.Pause != 0 {
			spanStart = spanStart.Add(w.Pause)
		}

		var attrs []attribute.KeyValue
		for a := 0; a < w.Attributes; a++ {
			key := fmt.Sprintf("attr_%02d", w.attrKeyNo)
			val := fmt.Sprintf("val_%02d", w.attrValNo)
			attrs = append(attrs, attribute.String(key, val))
			w.attrKeyNo = (w.attrKeyNo + 1) % w.AttrKeys
			w.attrValNo = (w.attrValNo + 1) % w.AttrValues
		}

		_, child := tracer.Start(
			ctx,
			fmt.Sprintf("child-span-%02d", c),
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(attrs...),
			trace.WithTimestamp(spanStart),
		)

		spanEnd := spanStart.Add(time.Duration(c)*time.Microsecond + time.Duration(w.spanInterval)*time.Microsecond)
		child.End(trace.WithTimestamp(spanEnd))

		lastEnd = spanEnd.Add(w.Pause)

		w.spanInterval++
		// Can be controlled,When will it be reset
		if w.spanInterval > 123 {
			w.spanInterval = 1
		}
	}

	return lastEnd
}
